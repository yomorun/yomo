package core

import (
	"github.com/quic-go/quic-go"
)

// A Listener for incoming connections
type Listener interface {
	quic.Listener
	// Name listerner's name
	Name() string
	// Versions
	Versions() []string
}
