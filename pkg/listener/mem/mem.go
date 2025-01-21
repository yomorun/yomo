// Package mem provides a memory implementation of yomo.FrameConn.
package mem

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/yomorun/yomo/core/frame"
)

// FrameConn is an implements of FrameConn,
// It transmits frames upon the golang channel.
type FrameConn struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	rCh    chan frame.Frame
	wCh    chan frame.Frame
}

var _ frame.Conn = &FrameConn{}

// NewFrameConn creates FrameConn from read write channel.
func NewFrameConn(ctx context.Context) *FrameConn {
	return newFrameConn(ctx, make(chan frame.Frame), make(chan frame.Frame))
}

// newFrameConn creates FrameConn from read write channel.
func newFrameConn(ctx context.Context, rCh, wCh chan frame.Frame) *FrameConn {
	ctx, cancel := context.WithCancelCause(ctx)

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
	if err := p.WriteFrame(hf); err != nil {
		return err
	}

	first, err := p.ReadFrame()
	if err != nil {
		return err
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
	select {
	case <-p.ctx.Done():
		return nil
	default:
		p.cancel(frame.NewErrConnClosed(false, errString))
	}
	return nil
}

// ReadFrame reads a frame. it usually be called in a for-loop.
func (p *FrameConn) ReadFrame() (frame.Frame, error) {
	select {
	case f := <-p.rCh:
		return f, nil
	case <-p.ctx.Done():
		return nil, context.Cause(p.ctx)
	}
}

// WriteFrame writes a frame to connection.
func (p *FrameConn) WriteFrame(f frame.Frame) error {
	select {
	case p.wCh <- f:
		return nil
	case <-p.ctx.Done():
		return context.Cause(p.ctx)
	}
}

// Listener listens a net.PacketConn and accepts connections.
type Listener struct {
	ctx    context.Context
	cancel context.CancelFunc
	conns  chan *FrameConn
}

// Listen returns a Listener that can accept connections.
func Listen() *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	l := &Listener{
		ctx:    ctx,
		cancel: cancel,
		conns:  make(chan *FrameConn, 10),
	}

	return l
}

func (l *Listener) Dial() (*FrameConn, error) {
	var (
		rCh = make(chan frame.Frame)
		wCh = make(chan frame.Frame)
	)
	conn := newFrameConn(l.ctx, rCh, wCh)

	select {
	case <-l.ctx.Done():
		return nil, l.ctx.Err()
	case l.conns <- conn:
		return conn, nil
	}
}

// Accept accepts FrameConns.
func (l *Listener) Accept(ctx context.Context) (frame.Conn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case c := <-l.conns:
			conn := &FrameConn{
				ctx:    c.ctx,
				cancel: c.cancel,
				// swap rCh and wCh for bidirectional
				rCh: c.wCh,
				wCh: c.rCh,
			}
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
