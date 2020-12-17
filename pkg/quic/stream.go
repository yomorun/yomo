package quic

import "io"

// Stream is the QUIC stream
type Stream interface {
	ReceiveStream
	SendStream
}

// ReceiveStream is an unidirectional Receive Stream.
type ReceiveStream interface {
	io.Reader
}

// A SendStream is an unidirectional Send Stream.
type SendStream interface {
	io.Writer
	io.Closer
}
