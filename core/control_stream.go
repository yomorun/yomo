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

type ControlStream interface {
	CloseStream(streamID string, errString string) error
	ReceiveStreamClose() (streamID string, errString string, err error)

	CloseWithError(code uint64, errString string) error
}

type ServerControlStream interface {
	ControlStream

	VerifyAuthentication(verifyFunc func(auth.Object) (bool, error)) error
	AcceptStream(context.Context) (DataStream, error)
}

type ClientControlStream interface {
	ControlStream

	Authenticate(*auth.Credential) error
	OpenStream(context.Context, *frame.HandshakeFrame) (DataStream, error)
}

var _ ServerControlStream = &serverControlStream{}

type serverControlStream struct {
	qconn  quic.Connection
	stream frame.ReadWriter
}

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
		_, err = stream.Write(frame.NewHandshakeAckFrame().Encode())
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
		return fmt.Errorf("yomo: read unexcept frame during waiting authentication, frame readed: %s", received.Type().String())
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

	_, ok := first.(*frame.HandshakeAckFrame)
	if !ok {
		return fmt.Errorf("yomo: data stream read first frame should be HandshakeAckFrame, but got %s", first.Type().String())
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
