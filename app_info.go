package yomo

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

type appInfo struct{}

func (a *appInfo) Encode() []byte {
	return nil
}

type appInfoBuilder struct {
	a *appInfo
}

func newAppInfoBuilder() core.AppInfoBuilder {
	return &appInfoBuilder{
		a: &appInfo{},
	}
}

func (builder *appInfoBuilder) Build(f *frame.HandshakeFrame) (core.AppInfo, error) {
	return builder.a, nil
}

func (builder *appInfoBuilder) Decode(buf []byte) (core.AppInfo, error) {
	return builder.a, nil
}
