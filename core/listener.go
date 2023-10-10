package core

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
)

// FrameConn is a connection that transmits data in frame format.
// It transmits frames upon the first stream from a QUIC connection.
type FrameConn struct {
	ctx       context.Context
	ctxCancel context.CancelCauseFunc
	frameCh   chan frame.Frame
	streamCh  chan io.Reader
	conn      quic.Connection
	stream    quic.Stream
	codec     frame.Codec
	prw       frame.PacketReadWriter
}

// DialAddr dials the given address and returns a new FrameConn.
func DialAddr(
	ctx context.Context,
	addr string,
	codec frame.Codec, prw frame.PacketReadWriter,
	tlsConfig *tls.Config, quicConfig *quic.Config,
) (*FrameConn, error) {
	qconn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}

	stream, err := qconn.OpenStream()
	if err != nil {
		return nil, err
	}

	return newFrameConn(qconn, stream, codec, prw), nil
}

func newFrameConn(
	qconn quic.Connection, stream quic.Stream,
	codec frame.Codec, prw frame.PacketReadWriter,
) *FrameConn {
	ctx, ctxCancel := context.WithCancelCause(context.Background())

	conn := &FrameConn{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		frameCh:   make(chan frame.Frame),
		streamCh:  make(chan io.Reader),
		conn:      qconn,
		stream:    stream,
		codec:     codec,
		prw:       prw,
	}

	go conn.framing()
	go conn.streaming()

	return conn
}

// YomoCloseErrorCode is the error code for close quic Connection for yomo.
// If the Connection implemented by quic is closed, the quic ApplicationErrorCode is always 0x13.
const YomoCloseErrorCode = quic.ApplicationErrorCode(0x13)

// Context returns FrameConn.Context.
// The Context be used to manage the lifecycle of connection and
// retrive error in `context.Cause(conn.Context())`
func (p *FrameConn) Context() context.Context {
	return p.ctx
}

// RemoteAddr returns the remote address of connection.
func (p *FrameConn) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}

// ErrConnectionClosed is returned when the connection is closed.
// If the connection is closed, both the stream and the connection will receive this error.
type ErrConnectionClosed struct {
	Message string
}

// Error implements the error interface and returns the reason why the connection was closed.
func (e *ErrConnectionClosed) Error() string {
	return e.Message
}

// CloseWithError closes the connection with an error message.
// If the connection is closed, it becomes unavailable.
func (p *FrameConn) CloseWithError(errString string) error {
	select {
	case <-p.ctx.Done():
		return nil
	default:
	}
	p.ctxCancel(&ErrConnectionClosed{errString})

	// _ = p.stream.Close()
	return p.conn.CloseWithError(YomoCloseErrorCode, errString)
}

func (p *FrameConn) framing() {
	for {
		fType, b, err := p.prw.ReadPacket(p.stream)
		if err != nil {
			p.ctxCancel(convertErrorToConnectionClosed(err))
			return
		}

		f, err := frame.NewFrame(fType)
		if err != nil {
			p.ctxCancel(convertErrorToConnectionClosed(err))
			return
		}

		if err := p.codec.Decode(b, f); err != nil {
			p.ctxCancel(convertErrorToConnectionClosed(err))
			return
		}

		p.frameCh <- f
	}
}

func convertErrorToConnectionClosed(err error) error {
	if se := new(quic.ApplicationError); errors.As(err, &se) {
		return &ErrConnectionClosed{se.ErrorMessage}
	}
	return err
}

// ReadFrame returns a channel from which frames can be received.
func (p *FrameConn) ReadFrame() <-chan frame.Frame {
	return p.frameCh
}

func (p *FrameConn) streaming() {
	for {
		reader, err := p.conn.AcceptUniStream(p.ctx)
		if err != nil {
			p.ctxCancel(convertErrorToConnectionClosed(err))
			return
		}
		p.streamCh <- reader
	}
}

// AcceptStream returns a channel from which streams can be received.
func (p *FrameConn) AcceptStream() <-chan io.Reader {
	return p.streamCh
}

// WriteFrame writes a frame to connection.
func (p *FrameConn) WriteFrame(f frame.Frame) error {
	select {
	case <-p.ctx.Done():
		return context.Cause(p.ctx)
	default:
	}

	b, err := p.codec.Encode(f)
	if err != nil {
		return err
	}

	return p.prw.WritePacket(p.stream, f.Type(), b)
}

// OpenStream opens a stream from connection.
func (p *FrameConn) OpenStream() (io.WriteCloser, error) {
	select {
	case <-p.ctx.Done():
		return nil, context.Cause(p.ctx)
	default:
	}

	return p.conn.OpenUniStream()
}

// Listener listens a net.PacketConn and accepts connections.
type Listener struct {
	underlying *quic.Listener
	codec      frame.Codec
	prw        frame.PacketReadWriter
}

// Listen returns a Listener.
func Listen(
	conn net.PacketConn,
	codec frame.Codec, prw frame.PacketReadWriter,
	tlsConfig *tls.Config, quicConfig *quic.Config,
) (*Listener, error) {
	ql, err := quic.Listen(conn, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		underlying: ql,
		codec:      codec,
		prw:        prw,
	}

	return listener, err
}

// ListenAddr listens an address and returns a new Listener.
func ListenAddr(
	addr string,
	codec frame.Codec, prw frame.PacketReadWriter,
	tlsConfig *tls.Config, quicConfig *quic.Config,
) (*Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	return Listen(conn, codec, prw, tlsConfig, quicConfig)
}

// Accept accepts FrameConns.
func (listener *Listener) Accept(ctx context.Context) (*FrameConn, error) {
	qconn, err := listener.underlying.Accept(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := qconn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	return newFrameConn(qconn, stream, listener.codec, listener.prw), nil
}

// Close closes listener.
// If listener be closed, all connection receive quic application error that code=0, message="".
func (listener *Listener) Close() error {
	return listener.underlying.Close()
}
