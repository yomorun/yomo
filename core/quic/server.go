package quic

import (
	"context"
)

// Server is the QUIC server.
type Server interface {
	// SetHandler sets QUIC callbacks.
	SetHandler(handler ServerHandler)

	// ListenAndServe starts listening on UDP network address addr and
	// serves incoming packets.
	ListenAndServe(ctx context.Context, addr string) error

	// Close the server. All active sessions will be closed.
	Close() error
}

// ServerHandler defines interface to handle the QUIC stream callbacks.
type ServerHandler interface {
	// Listen is the callback function when the QUIC server listening on a given address.
	Listen() error
	// Read is the callback function when the QUIC server receiving a new stream.
	Read(addr string, sess Session, st Stream) error
}

// NewServer inits the default implementation of QUIC server.
func NewServer(handler ServerHandler) Server {
	server := &quicGoServer{}
	server.SetHandler(handler)
	return server
}
