package core

import "github.com/yomorun/yomo/core/frame"

// MetaData is used for storing extra info of the application
type MetaData interface {
	// Encode is the serialize method
	Encode() []byte
}

// MetaDataBuilder is the builder of MetaData
type MetaDataBuilder interface {
	// Build will return an MetaData instance according to the handshake frame passed in
	Build(f *frame.HandshakeFrame) (MetaData, error)
	// Decode is the deserialize method
	Decode(buf []byte) (MetaData, error)
}
