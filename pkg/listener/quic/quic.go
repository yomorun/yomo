// Package yquic provides a quic implementation of yomo.FrameConn.
package yquic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
)

// FrameConn is an implements of FrameConn,
// It transmits frames upon the first stream from a QUIC connection.
type FrameConn struct {
	frameCh chan frame.Frame
	conn    quic.Connection
	stream  quic.Stream
	codec   frame.Codec
	prw     frame.PacketReadWriter
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

	conn := &FrameConn{
		frameCh: make(chan frame.Frame),
		conn:    qconn,
		stream:  stream,
		codec:   codec,
		prw:     prw,
	}

	return conn
}

// Context returns the context of the connection.
func (p *FrameConn) Context() context.Context {
	return p.conn.Context()
}

// RemoteAddr returns the remote address of connection.
func (p *FrameConn) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}

// LocalAddr returns the local address of connection.
func (p *FrameConn) LocalAddr() net.Addr {
	return p.conn.LocalAddr()
}

// CloseWithError closes the connection.
// After calling CloseWithError, ReadFrame and WriteFrame will return frame.ErrConnClosed error.
func (p *FrameConn) CloseWithError(errString string) error {
	// _ = p.stream.Close()

	// After closing the quic connection, the stream will receive
	// an quic.ApplicationError which error code is 0x13 (YomoCloseErrorCode).
	// If ReadFrame and WriteFrame encounter this error, that means the connection is closed.
	return p.conn.CloseWithError(YomoCloseErrorCode, errString)
}

func handleError(err error) error {
	if se := new(quic.ApplicationError); errors.As(err, &se) {
		// If the error code is 0, it means the listener is be closed.
		if se.ErrorCode == 0 && se.ErrorMessage == "" {
			return frame.NewErrConnClosed(true, "yomo: listener closed")
		}
		// If the error code is 0x13 (YomoCloseErrorCode), it means the connection is closed by remote or local.
		if se.ErrorCode == YomoCloseErrorCode {
			return frame.NewErrConnClosed(se.Remote, se.ErrorMessage)
		}
	}
	// Other errors are all unexcepted error, return it directly.
	return err
}

// ReadFrame reads a frame. it usually be called in a for-loop.
func (p *FrameConn) ReadFrame() (frame.Frame, error) {
	fType, b, err := p.prw.ReadPacket(p.stream)
	if err != nil {
		return nil, handleError(err)
	}
	f, err := frame.NewFrame(fType)
	if err != nil {
		return nil, err
	}
	if err := p.codec.Decode(b, f); err != nil {
		return nil, err
	}
	return f, nil
}

// WriteFrame writes a frame to connection.
func (p *FrameConn) WriteFrame(f frame.Frame) error {
	b, err := p.codec.Encode(f)
	if err != nil {
		return err
	}
	if err := p.prw.WritePacket(p.stream, f.Type(), b); err != nil {
		return handleError(err)
	}
	return nil
}

// Listener listens a net.PacketConn and accepts connections.
type Listener struct {
	underlying *quic.Listener
	codec      frame.Codec
	prw        frame.PacketReadWriter
}

// Listen returns a quic Listener that can accept connections.
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
func (listener *Listener) Accept(ctx context.Context) (frame.Conn, error) {
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

// YomoCloseErrorCode is the error code for close quic Connection for yomo.
// If the Connection implemented by quic is closed, the quic ApplicationErrorCode is always 0x13.
const YomoCloseErrorCode = quic.ApplicationErrorCode(0x13)
