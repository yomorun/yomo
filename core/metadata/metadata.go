// Package metadata defines `Metadata` and the `Builder`.
package metadata

import "github.com/yomorun/yomo/core/frame"

// Metadata is used for storing extra info of the application.
type Metadata interface {
	// Encode is the serialize method,
	// That represents the Metadata can be transmited.
	Encode() []byte
}

// Builder is the builder of Metadata.
// the metadata usually be built from `HandshakeFrame`,
// and It can be decode as byte array for io transmission.
type Builder interface {
	// Build returns an Metadata instance according to the handshake frame passed in.
	Build(f *frame.HandshakeFrame) (Metadata, error)
	// Decode is the deserialize method
	Decode(buf []byte) (Metadata, error)
}
