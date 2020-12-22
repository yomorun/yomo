package quic

import "context"

// Client is the QUIC client.
type Client interface {
	// CreateStream creates a bidirectional stream.
	CreateStream(ctx context.Context) (Stream, error)
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
