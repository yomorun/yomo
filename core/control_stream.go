// Package core defines the core interfaces of yomo.
package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/id"
	"golang.org/x/exp/slog"
)

// ErrHandshakeRejected be returned when a stream be rejected after sending a handshake.
// It contains the streamID and the error. It is used in AcceptStream scope.
type ErrHandshakeRejected struct {
	Message  string
	StreamID string
}

// Error returns a string that represents the ErrHandshakeRejected error for the implementation of the error interface.
func (e ErrHandshakeRejected) Error() string {
	return fmt.Sprintf("yomo: handshake be rejected, streamID=%s, message=%s", e.StreamID, e.Message)
}

// ErrAuthenticateFailed be returned when client control stream authenticate failed.
type ErrAuthenticateFailed struct {
	ReasonFromeServer string
}

// Error returns a string that represents the ErrAuthenticateFailed error for the implementation of the error interface.
func (e ErrAuthenticateFailed) Error() string { return e.ReasonFromeServer }

// HandshakeFunc is used by server control stream to handle handshake.
// The returned metadata will be set for the DataStream that is being opened.
type HandshakeFunc func(*frame.HandshakeFrame) (metadata.Metadata, error)

// VerifyAuthenticationFunc is used by server control stream to verify authentication.
type VerifyAuthenticationFunc func(*frame.AuthenticationFrame) (metadata.Metadata, bool, error)

// ServerControlStream defines the struct of server-side control stream.
type ServerControlStream struct {
	id                 string
	conn               Connection
	broker             *Broker
	stream             frame.ReadWriteCloser
	handshakeFrameChan chan *frame.HandshakeFrame
	codec              frame.Codec
	packetReadWriter   frame.PacketReadWriter
	logger             *slog.Logger
}

// NewServerControlStream returns ServerControlStream from quic Connection and the first stream of this Connection.
func NewServerControlStream(
	conn Connection, stream ContextReadWriteCloser,
	codec frame.Codec, packetReadWriter frame.PacketReadWriter,
	broker *Broker,
	logger *slog.Logger,
) *ServerControlStream {
	if logger == nil {
		logger = ylog.Default()
	}

	controlStream := &ServerControlStream{
		id:                 id.New(),
		conn:               conn,
		broker:             broker,
		stream:             NewFrameStream(stream, codec, packetReadWriter),
		handshakeFrameChan: make(chan *frame.HandshakeFrame, 10),
		codec:              codec,
		packetReadWriter:   packetReadWriter,
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
			ss.conn.CloseWithError(err.Error())
			return
		}
		switch ff := f.(type) {
		case *frame.HandshakeFrame:
			ss.handshakeFrameChan <- ff
		case *frame.ObserveFrame:
			ss.broker.Observe(ff.Tag, ss)
		default:
			ss.logger.Debug("control stream read unexpected frame", "frame_type", f.Type().String())
		}
	}
}

// OpenStream reveives a HandshakeFrame from control stream and handle it in the function passed in.
// if handler returns nil, will return a DataStream and nil,
// if handler returns an error, will return nil and the error.
func (ss *ServerControlStream) OpenStream(ctx context.Context, handshakeFunc HandshakeFunc) (DataStream, error) {
	ff, ok := <-ss.handshakeFrameChan
	if !ok {
		return nil, io.EOF
	}
	md, err := handshakeFunc(ff)
	if err != nil {
		ss.stream.WriteFrame(&frame.HandshakeRejectedFrame{
			ID:      ff.ID,
			Message: err.Error(),
		})
		return nil, err
	}

	stream, err := ss.conn.OpenStream()
	if err != nil {
		return nil, err
	}
	b, err := ss.codec.Encode(&frame.HandshakeAckFrame{
		StreamID: ff.ID,
	})
	if err != nil {
		return nil, err
	}
	_, err = stream.Write(b)
	if err != nil {
		return nil, err
	}
	dataStream := newDataStream(
		ff.Name,
		ff.ID,
		StreamType(ff.StreamType),
		md,
		ff.ObserveDataTags,
		NewFrameStream(stream, ss.codec, ss.packetReadWriter),
	)
	return dataStream, nil
}

func (ss *ServerControlStream) ID() string { return ss.id }

func (ss *ServerControlStream) AcceptUniStream(ctx context.Context) (io.ReadCloser, error) {
	return ss.conn.AcceptUniStream(ctx)
}

func (ss *ServerControlStream) OpenUniStream() (io.WriteCloser, error) {
	return ss.conn.OpenUniStream()
}

// CloseWithError closes the server-side control stream.
func (ss *ServerControlStream) CloseWithError(errString string) error {
	return ss.conn.CloseWithError(errString)
}

// VerifyAuthentication verify the Authentication from client side.
func (ss *ServerControlStream) VerifyAuthentication(verifyFunc VerifyAuthenticationFunc) (metadata.Metadata, error) {
	first, err := ss.stream.ReadFrame()
	if err != nil {
		return nil, err
	}

	received, ok := first.(*frame.AuthenticationFrame)
	if !ok {
		errString := fmt.Sprintf("authentication failed: read unexcepted frame, frame read: %s", received.Type().String())
		ss.CloseWithError(errString)
		return nil, errors.New(errString)
	}

	md, ok, err := verifyFunc(received)
	if err != nil {
		return md, err
	}
	if !ok {
		errString := fmt.Sprintf("authentication failed: client credential name is %s", received.AuthName)
		ss.CloseWithError(errString)
		return md, errors.New(errString)
	}
	if err := ss.stream.WriteFrame(&frame.AuthenticationAckFrame{ID: ss.id}); err != nil {
		return md, err
	}

	// create a goroutinue to continuous read frame after verify authentication successful.
	go ss.readFrameLoop()

	return md, nil
}

// ClientControlStream is the struct that defines the methods for client-side control stream.
type ClientControlStream struct {
	ctx             context.Context
	conn            Connection
	stream          frame.ReadWriteCloser
	metadataDecoder metadata.Decoder

	// encode and decode the frame
	codec            frame.Codec
	packetReadWriter frame.PacketReadWriter

	// id is he connectionID be assigned by server.
	id string

	// mu protect handshakeFrames
	mu              sync.Mutex
	handshakeFrames map[string]*frame.HandshakeFrame

	handshakeRejectedFrameChan chan *frame.HandshakeRejectedFrame
	acceptStreamResultChan     chan acceptStreamResult
	logger                     *slog.Logger
}

// RequestObserve requests server to observe a tag.
func (cs *ClientControlStream) RequestObserve(tag string) error {
	f := &frame.ObserveFrame{
		Tag: tag,
	}
	return cs.stream.WriteFrame(f)
}

// ID returns the ID of controlStream which is assigned by server.
func (cs *ClientControlStream) ID() string { return cs.id }

// Peer returns the Peer that work for creating writer stream and observing stream.
func (cs *ClientControlStream) Peer() *Peer {
	return NewPeer(cs, FillObserveWriter(cs.codec, cs.packetReadWriter))
}

// OpenUniStream opens a Writer.
func (cs *ClientControlStream) OpenUniStream() (io.WriteCloser, error) {
	return cs.conn.OpenUniStream()
}

// AcceptUniStream accepts a Reader.
func (cs *ClientControlStream) AcceptUniStream(ctx context.Context) (io.ReadCloser, error) {
	return cs.conn.AcceptUniStream(ctx)
}

// OpenClientControlStream opens ClientControlStream from addr.
func OpenClientControlStream(
	ctx context.Context, addr string,
	tlsConfig *tls.Config, quicConfig *quic.Config,
	metadataDecoder metadata.Decoder,
	codec frame.Codec, packetReadWriter frame.PacketReadWriter,
	logger *slog.Logger,
) (*ClientControlStream, error) {

	conn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}
	stream0, err := conn.OpenStream()
	if err != nil {
		return nil, err
	}

	cs := NewClientControlStream(ctx, &QuicConnection{conn}, stream0, codec, packetReadWriter, metadataDecoder, logger)

	return cs, nil
}

// NewClientControlStream returns ClientControlStream from quic Connection and the first stream form the Connection.
func NewClientControlStream(
	ctx context.Context, conn Connection, stream ContextReadWriteCloser,
	codec frame.Codec, packetReadWriter frame.PacketReadWriter,
	metadataDecoder metadata.Decoder, logger *slog.Logger) *ClientControlStream {

	controlStream := &ClientControlStream{
		ctx:                        ctx,
		conn:                       conn,
		stream:                     NewFrameStream(stream, codec, packetReadWriter),
		codec:                      codec,
		packetReadWriter:           packetReadWriter,
		metadataDecoder:            metadataDecoder,
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
			cs.conn.CloseWithError(err.Error())
			return
		}
		switch ff := f.(type) {
		case *frame.HandshakeRejectedFrame:
			cs.handshakeRejectedFrameChan <- ff
		default:
			cs.logger.Debug("control stream read unexcepted frame", "frame_type", f.Type().String())
		}
	}
}

// Authenticate sends the provided credential to the server's control stream to authenticate the client.
// There will return `ErrAuthenticateFailed` if authenticate failed.
func (cs *ClientControlStream) Authenticate(cred *auth.Credential) error {
	af := &frame.AuthenticationFrame{
		AuthName:    cred.Name(),
		AuthPayload: cred.Payload(),
	}
	if err := cs.stream.WriteFrame(af); err != nil {
		return err
	}
	received, err := cs.stream.ReadFrame()
	if err != nil {
		if qerr := new(quic.ApplicationError); errors.As(err, &qerr) && strings.HasPrefix(qerr.ErrorMessage, "authentication failed") {
			return &ErrAuthenticateFailed{qerr.ErrorMessage}
		}
		return err
	}
	f, ok := received.(*frame.AuthenticationAckFrame)
	if !ok {
		return fmt.Errorf(
			"yomo: read unexpected frame during waiting authentication resp, frame read: %s",
			received.Type().String(),
		)
	}
	cs.id = f.ID

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

	return f.StreamID, nil
}

// Observe tells server that the client wants to observe specified tag.
func (cs *ClientControlStream) Observe(tag string) error {
	f := &frame.ObserveFrame{
		Tag: tag,
	}
	return cs.stream.WriteFrame(f)
}

// RequestStream sends a HandshakeFrame to the server's control stream to request a new data stream.
// If the handshake is successful, a DataStream will be returned by the AcceptStream() method.
func (cs *ClientControlStream) RequestStream(hf *frame.HandshakeFrame) error {
	err := cs.stream.WriteFrame(hf)

	if err != nil {
		return err
	}

	cs.mu.Lock()
	cs.handshakeFrames[hf.ID] = hf
	cs.mu.Unlock()

	return nil
}

// AcceptStream accepts a DataStream from the server if SendHandshake() has been called before.
// This method should be executed in a for-loop.
// If the handshake is rejected, an ErrHandshakeRejected error will be returned. This error does not represent
// a network error and the for-loop can continue.
func (cs *ClientControlStream) AcceptStream(ctx context.Context) (DataStream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-cs.ctx.Done():
		return nil, cs.ctx.Err()
	case reject := <-cs.handshakeRejectedFrameChan:
		cs.mu.Lock()
		delete(cs.handshakeFrames, reject.ID)
		cs.mu.Unlock()

		return nil, ErrHandshakeRejected{
			Message:  reject.Message,
			StreamID: reject.ID,
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
	quicStream, err := cs.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	fs := NewFrameStream(quicStream, cs.codec, cs.packetReadWriter)

	streamID, err := ackDataStream(fs)
	if err != nil {
		return nil, err
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	f, ok := cs.handshakeFrames[streamID]
	if !ok {
		return nil, errors.New("yomo: client control stream accept stream without send handshake")
	}

	// Unlike server-side data streams,
	// client-side data streams do not merge connection-level metadata and stream-level metadata.
	// Instead, they only contain stream-level metadata.
	md, err := cs.metadataDecoder.Decode(f.Metadata)
	if err != nil {
		return nil, err
	}

	return newDataStream(f.Name, f.ID, StreamType(f.StreamType), md, f.ObserveDataTags, fs), nil
}

// CloseWithError closes the client-side control stream.
func (cs *ClientControlStream) CloseWithError(errString string) error {
	cs.stream.Close()
	return cs.conn.CloseWithError(errString)
}

// FillObserveWriter fill the observe tag to the writer.
func FillObserveWriter(codec frame.Codec, packetReadWriter frame.PacketReadWriter) func(string, io.Writer) error {
	return func(tag string, w io.Writer) error {
		f := &frame.ObserveFrame{
			Tag: tag,
		}
		b, err := codec.Encode(f)
		if err != nil {
			return err
		}
		return packetReadWriter.WritePacket(w, f.Type(), b)
	}
}

// DrainObserveReader drains tag from the reader and returns the tag.
func DrainObserveReader(codec frame.Codec, packetReadWriter frame.PacketReadWriter) func(io.Reader) (string, error) {
	return func(r io.Reader) (tag string, err error) {
		ft, b, err := packetReadWriter.ReadPacket(r)
		if err != nil {
			return "", err
		}
		if ft != frame.TypeObserveFrame {
			return "", errors.New("read unexpected frame")
		}

		f, err := frame.NewFrame(ft)
		if err != nil {
			return "", err
		}

		if err := codec.Decode(b, f); err != nil {
			return "", err
		}

		return f.(*frame.ObserveFrame).Tag, nil
	}
}
