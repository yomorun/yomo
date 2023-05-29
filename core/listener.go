package core

import (
	"context"
	"net"
)

// A Listener for incoming connections
type Listener interface {
	// Close the server. All active connections will be closed.
	Close() error
	// Addr returns the local network addr that the server is listening on.
	Addr() net.Addr
	// Accept returns new connections. It should be called in a loop.
	Accept(context.Context) (Connection, error)
}

// A Connection is a connection between two peers.
type Connection interface {
	// LocalAddr returns the local address.
	LocalAddr() string
	// RemoteAddr returns the address of the peer.
	RemoteAddr() string
	// OpenStream opens a new bidirectional QUIC stream.
	OpenStream() (ContextReadWriteCloser, error)
	// AcceptStream returns the next stream opened by the peer, blocking until one is available.
	// If the connection was closed due to a timeout, the error satisfies the net.Error interface, and Timeout() will be true.
	AcceptStream(context.Context) (ContextReadWriteCloser, error)
	// CloseWithError closes the connection with an error.
	CloseWithError(string) error
}
