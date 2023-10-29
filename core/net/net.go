package ynet

import (
	"context"
	"net"

	"github.com/yomorun/yomo/core/frame"
)

// Listener accepts FrameConns.
type Listener interface {
	// Accept accepts FrameConns.
	Accept(context.Context) (FrameConn, error)
	// Close closes listener,
	// If listener be closed, all FrameConn accepted will be unavailable.
	Close() error
}

// FrameConn is a connection that transmits data in frame format.
type FrameConn interface {
	// Context returns FrameConn.Context.
	// The Context can be used to manage the lifecycle of connection and
	// retrieve error using `context.Cause(conn.Context())` after calling `CloseWithError()`.
	Context() context.Context
	// WriteFrame writes a frame to connection.
	WriteFrame(frame.Frame) error
	// ReadFrame returns a channel from which frames can be received.
	ReadFrame() (frame.Frame, error)
	// RemoteAddr returns the remote address of connection.
	RemoteAddr() net.Addr
	// LocalAddr returns the local address of connection.
	LocalAddr() net.Addr
	// CloseWithError closes the connection with an error message.
	// It will be unavailable if the connection is closed. the error message should be written to the conn.Context().
	CloseWithError(string) error
}
