package quic

import "github.com/lucas-clemente/quic-go"

// Stream is the QUIC stream.
type Stream interface {
	quic.Stream
}

// Session is the QUIC session.
type Session interface {
	quic.Session
}
