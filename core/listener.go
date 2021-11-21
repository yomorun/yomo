package core

import (
	"crypto/tls"
	"net"

	"github.com/lucas-clemente/quic-go"
)

// A Listener for incoming connections
type Listener interface {
	quic.Listener
	// Name listerner's name
	Name() string
	// Listen listen incoming connections
	Listen(conn net.PacketConn, tlsConfig *tls.Config, quicConfig *quic.Config) error
	// Versions
	Versions() []string
}
