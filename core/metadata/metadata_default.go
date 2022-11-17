// Package metadata provides a default implements of `Metadata`.
<<<<<<< HEAD
=======

>>>>>>> f088ffd (style: fix code style issure from codacy)
package metadata

import (
	"github.com/yomorun/yomo/core/frame"
)

var _ Metadata = &Default{}

// Default returns an implement of `Metadata`,
// the default `Metadata` do not store anything.
type Default struct{}

// Encode returns nil, It indicates the application do not have metadata.
func (m *Default) Encode() []byte {
	return nil
}

type defaultBuilder struct {
	m *Default
}

// DefaultBuilder returns an implement of `Builder`,
// the default builder only return default `Metadata`, the default `Metadata`
// do not store anything.
func DefaultBuilder() Builder {
	return &defaultBuilder{
		m: &Default{},
	}
}

func (builder *defaultBuilder) Build(f *frame.HandshakeFrame) (Metadata, error) {
	return builder.m, nil
}

func (builder *defaultBuilder) Decode(buf []byte) (Metadata, error) {
	return builder.m, nil
}
