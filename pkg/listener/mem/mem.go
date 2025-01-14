// Package mem provides a memory implementation of yomo.FrameConn.
package mem

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/yomorun/yomo/core/frame"
)

// FrameConn is an implements of FrameConn,
// It transmits frames upon the golang channel.
type FrameConn struct {
	ctx    context.Context
	cancel context.CancelFunc
	errMsg atomic.Value
	rCh    chan frame.Frame
	wCh    chan frame.Frame
}

var _ frame.Conn = &FrameConn{}

// newFrameConn creates FrameConn from read write channel.
func newFrameConn(ctx context.Context, rCh, wCh chan frame.Frame) *FrameConn {
	ctx, cancel := context.WithCancel(ctx)

	conn := &FrameConn{
		ctx:    ctx,
		cancel: cancel,
		rCh:    rCh,
		wCh:    wCh,
	}

	return conn
}

// Handshake sends a HandshakeFrame to the connection.
// This function should be called before ReadFrame or WriteFrame.
func (p *FrameConn) Handshake(hf *frame.HandshakeFrame) error {
	p.rCh <- hf

	first, ok := <-p.wCh
	if !ok {
		return nil
	}

	switch f := first.(type) {
	case *frame.HandshakeAckFrame:
		return nil
	case *frame.RejectedFrame:
		return errors.New(f.Message)
	default:
		return errors.New("unexpected frame")
	}
}

// Context returns the context of the connection.
func (p *FrameConn) Context() context.Context {
	return p.ctx
}

type memAddr struct {
	remote bool
}

func (m *memAddr) Network() string {
	return "mem"
}
func (m *memAddr) String() string {
	rs := "local"
	if m.remote {
		rs = "remote"
	}
	return fmt.Sprintf("mem://%s", rs)
}

// RemoteAddr returns the remote address of connection.
func (p *FrameConn) RemoteAddr() net.Addr {
	addr := &memAddr{
		remote: true,
	}
	return addr
}

// LocalAddr returns the local address of connection.
func (p *FrameConn) LocalAddr() net.Addr {
	addr := &memAddr{
		remote: false,
	}
	return addr
}

// CloseWithError closes the connection.
// After calling CloseWithError, ReadFrame and WriteFrame will return frame.ErrConnClosed error.
func (p *FrameConn) CloseWithError(errString string) error {
	if v := p.errMsg.Load(); v == nil {
		p.errMsg.Store(errString)
	}
	p.cancel()
	return nil
}

func (p *FrameConn) closeError() error {
	v, ok := p.errMsg.Load().(string)
	if ok {
		return frame.NewErrConnClosed(false, v)
	}
	return nil
}

// ReadFrame reads a frame. it usually be called in a for-loop.
func (p *FrameConn) ReadFrame() (frame.Frame, error) {
	select {
	case <-p.ctx.Done():
		return nil, p.closeError()
	case f, ok := <-p.rCh:
		if !ok {
			return nil, p.closeError()
		}
		return f, nil
	}
}

// WriteFrame writes a frame to connection.
func (p *FrameConn) WriteFrame(f frame.Frame) error {
	select {
	case <-p.ctx.Done():
		return p.closeError()
	case p.wCh <- f:
		return nil
	}
}

// Listener listens a net.PacketConn and accepts connections.
type Listener struct {
	ctx    context.Context
	cancel context.CancelFunc
	in     chan frame.Frame
	outCh  chan chan frame.Frame
}

// Listen returns a quic Listener that can accept connections.
func Listen(in chan frame.Frame) *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	l := &Listener{
		ctx:    ctx,
		cancel: cancel,
		in:     in,
		outCh:  make(chan chan frame.Frame),
	}

	return l
}

func (l *Listener) Dial() (*FrameConn, error) {
	ch := make(chan frame.Frame)
	select {
	case <-l.ctx.Done():
		return nil, l.ctx.Err()
	case l.outCh <- ch:
		conn := newFrameConn(l.ctx, l.in, ch)
		return conn, nil
	}
}

// Accept accepts FrameConns.
func (l *Listener) Accept(ctx context.Context) (frame.Conn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case ch, ok := <-l.outCh:
			if !ok {
				return nil, frame.NewErrConnClosed(false, "listener has been closed")
			}
			conn := newFrameConn(ctx, l.in, ch)
			return conn, nil
		}
	}
}

// Close closes listener.
// If listener be closed, all connection receive quic application error that code=0, message="".
func (l *Listener) Close() error {
	l.cancel()
	return nil
}
