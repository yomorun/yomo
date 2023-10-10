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

type FrameConnection struct {
	ctx       context.Context
	ctxCancel context.CancelCauseFunc
	frameCh   chan frame.Frame
	streamCh  chan io.Reader
	conn      quic.Connection
	stream    quic.Stream
	codec     frame.Codec
	prw       frame.PacketReadWriter
}

func DialAddr(
	ctx context.Context,
	addr string,
	codec frame.Codec, prw frame.PacketReadWriter,
	tlsConfig *tls.Config, quicConfig *quic.Config,
) (*FrameConnection, error) {
	qconn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}

	stream, err := qconn.OpenStream()
	if err != nil {
		return nil, err
	}

	return newFrameConnection(qconn, stream, codec, prw), nil
}

func newFrameConnection(
	qconn quic.Connection, stream quic.Stream,
	codec frame.Codec, prw frame.PacketReadWriter,
) *FrameConnection {
	ctx, ctxCancel := context.WithCancelCause(context.Background())

	conn := &FrameConnection{
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

func (p *FrameConnection) Context() context.Context {
	return p.ctx
}

func (p *FrameConnection) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}

type ErrConnectionClosed struct {
	Message string
}

func (e *ErrConnectionClosed) Error() string {
	return e.Message
}

func (p *FrameConnection) CloseWithError(errString string) error {
	select {
	case <-p.ctx.Done():
		return nil
	default:
	}
	p.ctxCancel(&ErrConnectionClosed{errString})

	close(p.frameCh)
	close(p.streamCh)

	_ = p.stream.Close()
	return p.conn.CloseWithError(YomoCloseErrorCode, errString)
}

func IsConnectionClosed(err error) bool {
	// stream return io.EOF if the connection be closed.
	if err == io.EOF {
		return true
	}
	// connection receives `YomoCloseErrorCode` if called `CloseWithError()`.
	if se := new(quic.ApplicationError); errors.Is(err, se) || se.ErrorCode == YomoCloseErrorCode {
		return true
	}
	// context cancel with this error.
	if se := new(ErrConnectionClosed); errors.Is(err, se) {
		return true
	}
	return false
}

func (p *FrameConnection) framing() {
	for {
		fType, b, err := p.prw.ReadPacket(p.stream)
		if err != nil {
			p.ctxCancel(err)
			return
		}

		f, err := frame.NewFrame(fType)
		if err != nil {
			p.ctxCancel(err)
			return
		}

		if err := p.codec.Decode(b, f); err != nil {
			p.ctxCancel(err)
			return
		}

		p.frameCh <- f
	}
}

func (p *FrameConnection) ReadFrame() <-chan frame.Frame {
	return p.frameCh
}

func (p *FrameConnection) streaming() {
	for {
		reader, err := p.conn.AcceptUniStream(p.ctx)
		if err != nil {
			p.ctxCancel(err)
			return
		}
		p.streamCh <- reader
	}
}

func (p *FrameConnection) AcceptStream() <-chan io.Reader {
	return p.streamCh
}

func (p *FrameConnection) WriteFrame(f frame.Frame) error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
	}

	b, err := p.codec.Encode(f)
	if err != nil {
		return err
	}

	return p.prw.WritePacket(p.stream, f.Type(), b)
}

func (p *FrameConnection) OpenStream() (io.WriteCloser, error) {
	select {
	case <-p.ctx.Done():
		return nil, p.ctx.Err()
	default:
	}

	return p.conn.OpenUniStream()
}

type Listener struct {
	underlying *quic.Listener
	codec      frame.Codec
	prw        frame.PacketReadWriter
}

func Listen(
	conn net.PacketConn,
	tlsConfig *tls.Config, quicConfig *quic.Config,
	codec frame.Codec, prw frame.PacketReadWriter,
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

func ListenAddr(
	addr string,
	tlsConfig *tls.Config, quicConfig *quic.Config,
	codec frame.Codec, prw frame.PacketReadWriter,
) (*Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	return Listen(conn, tlsConfig, quicConfig, codec, prw)
}

func (listener *Listener) Accept(ctx context.Context) (*FrameConnection, error) {
	qconn, err := listener.underlying.Accept(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := qconn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	return newFrameConnection(qconn, stream, listener.codec, listener.prw), nil
}

func (listener *Listener) Close() error {
	return listener.underlying.Close()
}
