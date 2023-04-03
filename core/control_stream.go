// Package core defines the core interfaces of yomo.
package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
)

// ErrHandshakeRejected be returned when a stream be rejected after sending a handshake.
// It contains the streamID and the error. It is used in AcceptStream scope.
type ErrHandshakeRejected struct {
	Reason   string
	StreamID string
}

// Error returns a error string for the implementation of the error interface.
func (e *ErrHandshakeRejected) Error() string {
	return fmt.Sprintf("yomo: handshake be rejected, streamID=%s, reason=%s", e.StreamID, e.Reason)
}

// ControlStream defines the interface for controlling a stream.
type ControlStream interface {
	// CloseWithError closes the control stream.
	CloseWithError(code uint64, errString string) error
}

// ServerControlStream defines the interface of server side control stream.
type ServerControlStream interface {
	ControlStream

	// VerifyAuthentication verify the Authentication from client side.
	VerifyAuthentication(verifyFunc func(auth.Object) (bool, error)) error

	// OpenStream reveives a HandshakeFrame from control stream and handle it in the function be passed in.
	// if handler returns nil, There will return a DataStream and nil,
	// if handler returns an error, There will return a nil and the error,
	OpenStream(context.Context, func(*frame.HandshakeFrame) error) (DataStream, error)
}

// ClientControlStream is an interface that defines the methods for a client-side control stream.
type ClientControlStream interface {
	ControlStream

	// Authenticate sends the provided credential to the server's control stream to authenticate the client.
	Authenticate(*auth.Credential) error

	// RequestStream sends a HandshakeFrame to the server's control stream to request a new data stream.
	// If the handshake is successful, a DataStream will be returned by the AcceptStream() method.
	RequestStream(*frame.HandshakeFrame) error

	// AcceptStream accepts a DataStream from the server if SendHandshake() has been called before.
	// This method should be executed in a for-loop.
	// If the handshake is rejected, an ErrHandshakeRejected error will be returned. This error does not represent
	// a network error and the for-loop can continue.
	AcceptStream(context.Context) (DataStream, error)
}

var _ ServerControlStream = &serverControlStream{}

type serverControlStream struct {
	qconn              quic.Connection
	stream             frame.ReadWriter
	handshakeFrameChan chan *frame.HandshakeFrame
}

// NewServerControlStream returns ServerControlStream from quic Connection and the first stream of this Connection.
func NewServerControlStream(qconn quic.Connection, stream frame.ReadWriter) ServerControlStream {
	controlStream := &serverControlStream{
		qconn:              qconn,
		stream:             stream,
		handshakeFrameChan: make(chan *frame.HandshakeFrame),
	}

	return controlStream
}

func (ss *serverControlStream) continusReadFrame() {
	defer func() {
		close(ss.handshakeFrameChan)
	}()
	for {
		f, err := ss.stream.ReadFrame()
		if err != nil {
			ss.qconn.CloseWithError(0, err.Error())
			return
		}
		switch ff := f.(type) {
		case *frame.HandshakeFrame:
			ss.handshakeFrameChan <- ff
		default:
			ss.qconn.CloseWithError(0, fmt.Sprintf("yomo: server control stream read unexcepted frame %s", f.Type().String()))
			return
		}
	}
}

func (ss *serverControlStream) OpenStream(ctx context.Context, handshakeFunc func(*frame.HandshakeFrame) error) (DataStream, error) {
	ff, ok := <-ss.handshakeFrameChan
	if !ok {
		return nil, io.EOF
	}
	err := handshakeFunc(ff)
	if err != nil {
		ss.stream.WriteFrame(frame.NewHandshakeRejectFrame(ff.ID(), err.Error()))
		return nil, err
	}

	stream, err := ss.qconn.OpenStreamSync(ctx)
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
	)
	return dataStream, nil
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
		return fmt.Errorf("yomo: read unexcept frame while waiting for authentication, frame read: %s", received.Type().String())
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
	if err := ss.stream.WriteFrame(frame.NewAuthenticationRespFrame(true, "")); err != nil {
		return err
	}

	// create a goroutinue to continus read frame after verify authentication successful.
	go ss.continusReadFrame()

	return nil
}

var _ ClientControlStream = &clientControlStream{}

type clientControlStream struct {
	ctx    context.Context
	qconn  quic.Connection
	stream frame.ReadWriter
	// mu protect handshakeFrames
	mu                       sync.Mutex
	handshakeFrames          map[string]*frame.HandshakeFrame
	handshakeRejectFrameChan chan *frame.HandshakeRejectFrame
	acceptStreamResultChan   chan acceptStreamResult
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
	stream0, err := qconn.OpenStream()
	if err != nil {
		return nil, err
	}

	return NewClientControlStream(ctx, qconn, NewFrameStream(stream0)), nil
}

// NewClientControlStream returns ClientControlStream from quic Connection and the first stream form the Connection.
func NewClientControlStream(ctx context.Context, qconn quic.Connection, stream frame.ReadWriter) ClientControlStream {
	controlStream := &clientControlStream{
		ctx:                      ctx,
		qconn:                    qconn,
		stream:                   stream,
		handshakeFrames:          make(map[string]*frame.HandshakeFrame),
		handshakeRejectFrameChan: make(chan *frame.HandshakeRejectFrame),
		acceptStreamResultChan:   make(chan acceptStreamResult),
	}

	return controlStream
}

func (cs *clientControlStream) continusReadFrame() {
	defer func() {
		close(cs.handshakeRejectFrameChan)
	}()
	for {
		f, err := cs.stream.ReadFrame()
		if err != nil {
			cs.qconn.CloseWithError(0, err.Error())
			return
		}
		switch ff := f.(type) {
		case *frame.HandshakeRejectFrame:
			cs.handshakeRejectFrameChan <- ff
		default:
			cs.qconn.CloseWithError(0, fmt.Sprintf("yomo: server control stream read unexcepted frame %s", f.Type().String()))
		}
	}
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

	// create a goroutinue to continus read frame from server.
	go cs.continusReadFrame()
	// create an other goroutinue to continus accept stream from server.
	go cs.continusAcceptStream(cs.ctx)

	return nil
}

// ackDataStream drain HandshakeAckFrame from the Reader and return streamID and error.
func ackDataStream(stream frame.Reader) (string, error) {
	first, err := stream.ReadFrame()
	if err != nil {
		return "", err
	}

	f, ok := first.(*frame.HandshakeAckFrame)
	if !ok {
		return "", fmt.Errorf("yomo: data stream read first frame should be HandshakeAckFrame, but got %s", first.Type().String())
	}

	return f.StreamID(), nil
}

func (cs *clientControlStream) RequestStream(hf *frame.HandshakeFrame) error {
	err := cs.stream.WriteFrame(hf)

	if err != nil {
		return err
	}

	cs.mu.Lock()
	cs.handshakeFrames[hf.ID()] = hf
	cs.mu.Unlock()

	return nil
}

func (cs *clientControlStream) AcceptStream(ctx context.Context) (DataStream, error) {
	select {
	case reject := <-cs.handshakeRejectFrameChan:
		cs.mu.Lock()
		delete(cs.handshakeFrames, reject.StreamID())
		cs.mu.Unlock()

		return nil, &ErrHandshakeRejected{
			Reason:   reject.Reason(),
			StreamID: reject.StreamID(),
		}
	case result := <-cs.acceptStreamResultChan:
		if err := result.err; err != nil {
			return nil, err
		}

		cs.mu.Lock()
		delete(cs.handshakeFrames, result.stream.ID())
		cs.mu.Unlock()

		return result.stream, nil
	}
}

type acceptStreamResult struct {
	stream DataStream
	err    error
}

func (cs *clientControlStream) continusAcceptStream(ctx context.Context) {
	for {
		dataStream, err := cs.acceptStream(ctx)
		cs.acceptStreamResultChan <- acceptStreamResult{dataStream, err}
		if err != nil {
			return
		}
	}
}

func (cs *clientControlStream) acceptStream(ctx context.Context) (DataStream, error) {
	quicStream, err := cs.qconn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	streamID, err := ackDataStream(NewFrameStream(quicStream))
	if err != nil {
		return nil, err
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	f, ok := cs.handshakeFrames[streamID]
	if !ok {
		return nil, errors.New("yomo: client control stream accept stream without send handshake")
	}

	return newDataStream(
		f.Name(),
		f.ID(),
		StreamType(f.StreamType()),
		f.Metadata(),
		quicStream,
		f.ObserveDataTags(),
	), nil
}

func (cs *clientControlStream) CloseWithError(code uint64, errString string) error {
	return closeWithError(cs.qconn, code, errString)
}

func closeWithError(qconn quic.Connection, code uint64, errString string) error {
	return qconn.CloseWithError(
		quic.ApplicationErrorCode(code),
		errString,
	)
}
