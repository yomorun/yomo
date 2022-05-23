package yomo

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

type metadata struct{}

func (m *metadata) Encode() []byte {
	return nil
}

type metadataBuilder struct {
	m *metadata
}

func newMetadataBuilder() core.MetadataBuilder {
	return &metadataBuilder{
		m: &metadata{},
	}
}

func (builder *metadataBuilder) Build(f *frame.HandshakeFrame) (core.Metadata, error) {
	return builder.m, nil
}

func (builder *metadataBuilder) Decode(buf []byte) (core.Metadata, error) {
	return builder.m, nil
}
