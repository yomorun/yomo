// Package core defines the core interfaces of yomo.
package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
)

// ControlStream defines the interface for controlling a stream.
type ControlStream interface {
	// CloseStream notifies the peer's control stream to close the data stream with the given streamID and error message.
	CloseStream(streamID string, errString string) error
	// ReceiveStreamClose is received from the peer's control stream to close the data stream according to streamID and error message.
	ReceiveStreamClose() (streamID string, errString string, err error)
	// CloseWithError closes the control stream.
	CloseWithError(code uint64, errString string) error
}

// ServerControlStream defines the interface of server side control stream.
type ServerControlStream interface {
	ControlStream

	// VerifyAuthentication verify the Authentication from client side.
	VerifyAuthentication(verifyFunc func(auth.Object) (bool, error)) error
	// AcceptStream accepts data stream from the request of client.
	AcceptStream(context.Context) (DataStream, error)
}

// ClientControlStream defines the interface of client side control stream.
type ClientControlStream interface {
	ControlStream

	// Authenticate with credential, the credential will be sent to ServerControlStream to authenticate the client.
	Authenticate(*auth.Credential) error
	// OpenStream request a ServerControlStream to create a new data stream.
	OpenStream(context.Context, *frame.HandshakeFrame) (DataStream, error)
}

var _ ServerControlStream = &serverControlStream{}

type serverControlStream struct {
	qconn  quic.Connection
	stream frame.ReadWriter
}

// NewServerControlStream returns ServerControlStream from quic Connection and the first stream of this Connection.
func NewServerControlStream(qconn quic.Connection, stream frame.ReadWriter) ServerControlStream {
	return &serverControlStream{
		qconn:  qconn,
		stream: stream,
	}
}

func (ss *serverControlStream) ReceiveStreamClose() (streamID string, errReason string, err error) {
	return receiveStreamClose(ss.stream)
}

func (ss *serverControlStream) CloseStream(streamID string, errString string) error {
	return closeStream(ss.stream, streamID, errString)
}

func (ss *serverControlStream) AcceptStream(context.Context) (DataStream, error) {
	f, err := ss.stream.ReadFrame()
	if err != nil {
		return nil, err
	}

	switch ff := f.(type) {
	case *frame.HandshakeFrame:
		stream, err := ss.qconn.OpenStreamSync(context.Background())
		if err != nil {
			return nil, err
		}
		_, err = stream.Write(frame.NewHandshakeAckFrame(ff.ID()).Encode())
		if err != nil {
			return nil, err
		}
		dataStream := newDataStream(
			ff.Name(),
			ff.ID(),
			StreamType(ff.StreamType()),
			ff.Metadata(),
			stream,
			ff.ObserveDataTags(),
			ss,
		)
		return dataStream, nil
	default:
		return nil, fmt.Errorf("yomo: control stream read unexpected frame %s", f.Type())
	}
}

func (ss *serverControlStream) CloseWithError(code uint64, errString string) error {
	return closeWithError(ss.qconn, code, errString)
}

func (ss *serverControlStream) VerifyAuthentication(verifyFunc func(auth.Object) (bool, error)) error {
	first, err := ss.stream.ReadFrame()
	if err != nil {
		return err
	}
	received, ok := first.(*frame.AuthenticationFrame)
	if !ok {
		return fmt.Errorf("yomo: read unexpected frame while waiting for authentication, frame read: %s", received.Type().String())
	}
	ok, err = verifyFunc(received)
	if err != nil {
		return err
	}
	if !ok {
		return ss.stream.WriteFrame(
			frame.NewAuthenticationRespFrame(
				false,
				fmt.Sprintf("yomo: authentication failed, client credential name is %s", received.AuthName()),
			),
		)
	}
	return ss.stream.WriteFrame(frame.NewAuthenticationRespFrame(true, ""))
}

var _ ClientControlStream = &clientControlStream{}

type clientControlStream struct {
	qconn  quic.Connection
	stream frame.ReadWriter
}

// OpenClientControlStream opens ClientControlStream from addr.
func OpenClientControlStream(
	ctx context.Context, addr string,
	tlsConfig *tls.Config, quicConfig *quic.Config,
) (ClientControlStream, error) {
	qconn, err := quic.DialAddrContext(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}
	stream, err := qconn.OpenStream()
	if err != nil {
		return nil, err
	}

	return NewClientControlStream(qconn, NewFrameStream(stream)), nil
}

// NewClientControlStream returns ClientControlStream from quic Connection and the first stream form the Connection.
func NewClientControlStream(qconn quic.Connection, stream frame.ReadWriter) ClientControlStream {
	return &clientControlStream{
		qconn:  qconn,
		stream: stream,
	}
}

func (cs *clientControlStream) ReceiveStreamClose() (streamID string, errReason string, err error) {
	return receiveStreamClose(cs.stream)
}

func (cs *clientControlStream) CloseStream(streamID string, errString string) error {
	return closeStream(cs.stream, streamID, errString)
}

func (cs *clientControlStream) Authenticate(cred *auth.Credential) error {
	if err := cs.stream.WriteFrame(
		frame.NewAuthenticationFrame(cred.Name(), cred.Payload())); err != nil {
		return err
	}
	received, err := cs.stream.ReadFrame()
	if err != nil {
		return err
	}
	resp, ok := received.(*frame.AuthenticationRespFrame)
	if !ok {
		return fmt.Errorf(
			"yomo: read unexcept frame during waiting authentication resp, frame readed: %s",
			received.Type().String(),
		)
	}
	if !resp.OK() {
		return errors.New(resp.Reason())
	}
	return nil
}

// dataStreamAcked drain HandshakeAckFrame from stream.
func dataStreamAcked(stream DataStream) error {
	first, err := stream.ReadFrame()
	if err != nil {
		return err
	}

	f, ok := first.(*frame.HandshakeAckFrame)
	if !ok {
		return fmt.Errorf("yomo: data stream read first frame should be HandshakeAckFrame, but got %s", first.Type().String())
	}

	if f.StreamID() != stream.ID() {
		return fmt.Errorf("yomo: data stream ack exception, stream id did not match")
	}

	return nil
}

func (cs *clientControlStream) OpenStream(ctx context.Context, hf *frame.HandshakeFrame) (DataStream, error) {
	err := cs.stream.WriteFrame(frame.NewHandshakeFrame(
		hf.Name(),
		hf.ID(),
		hf.StreamType(),
		hf.ObserveDataTags(),
		hf.Metadata(),
	))

	if err != nil {
		return nil, err
	}

	quicStream, err := cs.qconn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	dataStream := newDataStream(
		hf.Name(),
		hf.ID(),
		StreamType(hf.StreamType()),
		hf.Metadata(),
		quicStream,
		hf.ObserveDataTags(),
		cs,
	)

	if err := dataStreamAcked(dataStream); err != nil {
		return nil, err
	}

	return dataStream, nil
}

func (cs *clientControlStream) CloseWithError(code uint64, errString string) error {
	return closeWithError(cs.qconn, code, errString)
}

func closeStream(controlStream frame.Writer, streamID string, errString string) error {
	f := frame.NewCloseStreamFrame(streamID, errString)
	return controlStream.WriteFrame(f)
}

func receiveStreamClose(controlStream frame.Reader) (streamID string, errString string, err error) {
	f, err := controlStream.ReadFrame()
	if err != nil {
		return "", "", err
	}
	ff, ok := f.(*frame.CloseStreamFrame)
	if !ok {
		return "", "", errors.New("yomo: control stream only transmit close stream frame")
	}
	return ff.StreamID(), ff.Reason(), nil
}

func closeWithError(qconn quic.Connection, code uint64, errString string) error {
	return qconn.CloseWithError(
		quic.ApplicationErrorCode(code),
		errString,
	)
}
