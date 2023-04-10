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
	"github.com/yomorun/yomo/core/ylog"
	"golang.org/x/exp/slog"
)

// ErrHandshakeRejected be returned when a stream be rejected after sending a handshake.
// It contains the streamID and the error. It is used in AcceptStream scope.
type ErrHandshakeRejected struct {
	Reason   string
	StreamID string
}

// Error returns a string that represents the ErrHandshakeRejected error for the implementation of the error interface.
func (e ErrHandshakeRejected) Error() string {
	return fmt.Sprintf("yomo: handshake be rejected, streamID=%s, reason=%s", e.StreamID, e.Reason)
}

// ErrAuthenticateFailed be returned when client control stream authenticate failed.
type ErrAuthenticateFailed struct {
	ReasonFromeServer string
}

// Error returns a string that represents the ErrAuthenticateFailed error for the implementation of the error interface.
func (e ErrAuthenticateFailed) Error() string { return e.ReasonFromeServer }

// ServerControlStream defines the struct of server-side control stream.
type ServerControlStream struct {
	qconn              quic.Connection
	stream             frame.ReadWriter
	handshakeFrameChan chan *frame.HandshakeFrame
	logger             *slog.Logger
}

// NewServerControlStream returns ServerControlStream from quic Connection and the first stream of this Connection.
func NewServerControlStream(qconn quic.Connection, stream frame.ReadWriter, logger *slog.Logger) *ServerControlStream {
	if logger == nil {
		logger = ylog.Default()
	}
	controlStream := &ServerControlStream{
		qconn:              qconn,
		stream:             stream,
		handshakeFrameChan: make(chan *frame.HandshakeFrame, 10),
		logger:             logger,
	}

	return controlStream
}

func (ss *ServerControlStream) readFrameLoop() {
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
			ss.logger.Debug("server control stream read unexcepted frame", "frame_type", f.Type().String())
		}
	}
}

// OpenStream reveives a HandshakeFrame from control stream and handle it in the function be passed in.
// if handler returns nil, There will return a DataStream and nil,
// if handler returns an error, There will return a nil and the error.
func (ss *ServerControlStream) OpenStream(ctx context.Context, handshakeFunc func(*frame.HandshakeFrame) error) (DataStream, error) {
	ff, ok := <-ss.handshakeFrameChan
	if !ok {
		return nil, io.EOF
	}
	err := handshakeFunc(ff)
	if err != nil {
		ss.stream.WriteFrame(frame.NewHandshakeRejectedFrame(ff.ID(), err.Error()))
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

// CloseWithError closes the server-side control stream.
func (ss *ServerControlStream) CloseWithError(code uint64, errString string) error {
	return closeWithError(ss.qconn, code, errString)
}

// VerifyAuthentication verify the Authentication from client side.
func (ss *ServerControlStream) VerifyAuthentication(verifyFunc func(auth.Object) (bool, error)) error {
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

	// create a goroutinue to continuous read frame after verify authentication successful.
	go ss.readFrameLoop()

	return nil
}

// ClientControlStream is the struct that defines the methods for client-side control stream.

type ClientControlStream struct {
	ctx    context.Context
	qconn  quic.Connection
	stream frame.ReadWriter
	// mu protect handshakeFrames
	mu                         sync.Mutex
	handshakeFrames            map[string]*frame.HandshakeFrame
	handshakeRejectedFrameChan chan *frame.HandshakeRejectedFrame
	acceptStreamResultChan     chan acceptStreamResult
	logger                     *slog.Logger
}

// OpenClientControlStream opens ClientControlStream from addr.
func OpenClientControlStream(
	ctx context.Context, addr string,
	tlsConfig *tls.Config, quicConfig *quic.Config,
	logger *slog.Logger,
) (*ClientControlStream, error) {
	qconn, err := quic.DialAddrContext(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}
	stream0, err := qconn.OpenStream()
	if err != nil {
		return nil, err
	}

	return NewClientControlStream(ctx, qconn, NewFrameStream(stream0), logger), nil
}

// NewClientControlStream returns ClientControlStream from quic Connection and the first stream form the Connection.
func NewClientControlStream(ctx context.Context, qconn quic.Connection, stream frame.ReadWriter, logger *slog.Logger) *ClientControlStream {
	if logger == nil {
		logger = ylog.Default()
	}
	controlStream := &ClientControlStream{
		ctx:                        ctx,
		qconn:                      qconn,
		stream:                     stream,
		handshakeFrames:            make(map[string]*frame.HandshakeFrame),
		handshakeRejectedFrameChan: make(chan *frame.HandshakeRejectedFrame, 10),
		acceptStreamResultChan:     make(chan acceptStreamResult, 10),
		logger:                     logger,
	}

	return controlStream
}

func (cs *ClientControlStream) readFrameLoop() {
	defer func() {
		close(cs.handshakeRejectedFrameChan)
	}()
	for {
		f, err := cs.stream.ReadFrame()
		if err != nil {
			cs.qconn.CloseWithError(0, err.Error())
			return
		}
		switch ff := f.(type) {
		case *frame.HandshakeRejectedFrame:
			cs.handshakeRejectedFrameChan <- ff
		default:
			cs.logger.Debug("client control stream read unexcepted frame", "frame_type", f.Type().String())
		}
	}
}

// Authenticate sends the provided credential to the server's control stream to authenticate the client.

func (cs *ClientControlStream) Authenticate(cred *auth.Credential) error {
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
		return &ErrAuthenticateFailed{resp.Reason()}
	}

	// create a goroutinue to continuous read frame from server.
	go cs.readFrameLoop()
	// create an other goroutinue to continuous accept stream from server.
	go cs.acceptStreamLoop(cs.ctx)

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

// RequestStream sends a HandshakeFrame to the server's control stream to request a new data stream.
// If the handshake is successful, a DataStream will be returned by the AcceptStream() method.
func (cs *ClientControlStream) RequestStream(hf *frame.HandshakeFrame) error {
	err := cs.stream.WriteFrame(hf)

	if err != nil {
		return err
	}

	cs.mu.Lock()
	cs.handshakeFrames[hf.ID()] = hf
	cs.mu.Unlock()

	return nil
}

// AcceptStream accepts a DataStream from the server if SendHandshake() has been called before.
// This method should be executed in a for-loop.
// If the handshake is rejected, an ErrHandshakeRejected error will be returned. This error does not represent
// a network error and the for-loop can continue.
func (cs *ClientControlStream) AcceptStream(ctx context.Context) (DataStream, error) {
	select {
	case reject := <-cs.handshakeRejectedFrameChan:
		cs.mu.Lock()
		delete(cs.handshakeFrames, reject.StreamID())
		cs.mu.Unlock()

		return nil, ErrHandshakeRejected{
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

func (cs *ClientControlStream) acceptStreamLoop(ctx context.Context) {
	for {
		dataStream, err := cs.acceptStream(ctx)
		cs.acceptStreamResultChan <- acceptStreamResult{dataStream, err}
		if err != nil {
			return
		}
	}
}

func (cs *ClientControlStream) acceptStream(ctx context.Context) (DataStream, error) {
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

// CloseWithError closes the client-side control stream.
func (cs *ClientControlStream) CloseWithError(code uint64, errString string) error {
	return closeWithError(cs.qconn, code, errString)
}

func closeWithError(qconn quic.Connection, code uint64, errString string) error {
	return qconn.CloseWithError(
		quic.ApplicationErrorCode(code),
		errString,
	)
}
