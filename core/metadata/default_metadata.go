package metadata

import (
	"github.com/yomorun/yomo/core/frame"
)

type Default struct{}

func (m *Default) Encode() []byte {
	return nil
}

type defaultBuilder struct {
	m *Default
}

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
