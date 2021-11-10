package core

import (
	"context"

	"github.com/lucas-clemente/quic-go"
)

// A Listener for incoming connections
type Listener interface {
	quic.Listener
	// Name listerner's name
	Name() string
	// Listen listen incoming connections
	Listen(ctx context.Context, addr string) error
	// Versions
	Versions() []string
}
