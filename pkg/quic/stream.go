package quic

import "github.com/lucas-clemente/quic-go"

// Stream is the QUIC stream
type Stream interface {
	quic.Stream
}
