package quic

import (
	"context"
)

// Client is the QUIC client.
type Client interface {
	// AcceptStream returns the next stream opened by the peer, blocking until one is available.
	// If the session was closed due to a timeout, the error satisfies
	// the net.Error interface, and Timeout() will be true.
	AcceptStream(ctx context.Context) (Stream, error)

	// AcceptUniStream returns the next unidirectional stream opened by the peer, blocking until one is available.
	// If the session was closed due to a timeout, the error satisfies
	// the net.Error interface, and Timeout() will be true.
	AcceptUniStream(ctx context.Context) (ReceiveStream, error)

	// CreateStream creates a bidirectional stream.
	CreateStream(ctx context.Context) (Stream, error)

	// CreateStream creates a unidirectional stream.
	CreateUniStream(ctx context.Context) (SendStream, error)

	// Close the QUIC client.
	Close() error
}

// NewClient inits the default implementation of QUIC client.
func NewClient(addr string) (Client, error) {
	client := &quicGoClient{}
	err := client.Connect(addr)

	if err != nil {
		return nil, err
	}
	return client, nil
}
