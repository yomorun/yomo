package yomo

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

type metaData struct{}

func (m *metaData) Encode() []byte {
	return nil
}

type metaDataBuilder struct {
	m *metaData
}

func newMetaDataBuilder() core.MetaDataBuilder {
	return &metaDataBuilder{
		m: &metaData{},
	}
}

func (builder *metaDataBuilder) Build(f *frame.HandshakeFrame) (core.MetaData, error) {
	return builder.m, nil
}

func (builder *metaDataBuilder) Decode(buf []byte) (core.MetaData, error) {
	return builder.m, nil
}
