// Package metadata defines `Metadata` and the `Decoder`.
package metadata

// Metadata is an interface used to store additional information about the application.
//
//	There are three types of metadata in yomo:
//	 1. Metadata from `Authentication.Authenticate()`, This is connection-level metadata.
//	 2. Metadata from the handshake, This is stream-level metadata.
//	 3. Metadata from the DataFrame, This is frame-level metadata.
//
// These types of metadata can be merged together to route the SFN.
type Metadata interface {
	// Encode encodes the metadata into a byte slice.
	Encode() ([]byte, error)
	// Merge defines the method for merging metadata from other source into the existing metadata.
	Merge(Metadata) Metadata
}

// Decoder is an interface that defines methods for decoding metadata.
// Implementations of this interface can be used to decode metadata to its binary representation.
type Decoder interface {
	// Decode decodes the given byte slice into metadata.
	Decode([]byte) (Metadata, error)
}
