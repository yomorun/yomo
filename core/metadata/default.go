// Package metadata provides a default implements of `Metadata` and `Encoder`.
package metadata

var _ Metadata = &Default{}

// Default returns an implement of `Metadata`, the default `Metadata` do not store anything.
type Default struct{}

// Merge do nothing.
func (m *Default) Merge(other Metadata) Metadata { return m }

// Encode returns empty byte slice.
func (m *Default) Encode() ([]byte, error) { return []byte{}, nil }

type defaultEncoder struct {
	m *Default
}

// DefaultDecoder returns the implement of `Codec`, Codec do nothing.
func DefaultDecoder() Decoder { return &defaultEncoder{&Default{}} }

func (encoder *defaultEncoder) Decode([]byte) (Metadata, error) { return encoder.m, nil }
