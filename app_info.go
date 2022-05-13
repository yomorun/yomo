package yomo

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

type appInfo struct{}

func (a *appInfo) Key() string {
	return ""
}

type appInfoBuilder struct{}

func newAppInfoBuilder() core.AppInfoBuilder {
	return &appInfoBuilder{}
}

func (a *appInfoBuilder) Build(f frame.Frame) (core.AppInfo, error) {
	return &appInfo{}, nil
}
