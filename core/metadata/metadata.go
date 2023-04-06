// Package metadata defines `Metadata` and the `Decoder`.
package metadata

// Metadata is an interface used to store additional information about the application.
//
//	There are three types of metadata in yomo:
//	 1. Metadata from `Authentication.Authenticate()`, This means that the metadata is built in the control stream.
//	 2. Metadata from the data stream.
//	 3. Metadata in DataFrame.
type Metadata interface {
	// Encode encodes the metadata into a byte slice.
	Encode() ([]byte, error)
	// Merge defines the method for merging metadata from other sources into the existing metadata.
	Merge(...Metadata) Metadata
}

// Decoder is an interface that defines methods for decoding metadata.
// Implementations of this interface can be used to decode metadata to its binary representation.
type Decoder interface {
	// Decode decodes the given byte slice into metadata.
	Decode([]byte) (Metadata, error)
}
