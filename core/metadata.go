package core

import "github.com/yomorun/yomo/core/frame"

// Metadata is used for storing extra info of the application
type Metadata interface {
	// Encode is the serialize method
	Encode() []byte
}

// MetadataBuilder is the builder of Metadata
type MetadataBuilder interface {
	// Build will return an Metadata instance according to the handshake frame passed in
	Build(f *frame.HandshakeFrame) (Metadata, error)
	// Decode is the deserialize method
	Decode(buf []byte) (Metadata, error)
}
