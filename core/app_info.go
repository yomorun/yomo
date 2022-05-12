package core

import "github.com/yomorun/yomo/core/frame"

// AppInfo is used for customizing extensions of an application.
type AppInfo interface {
	Key() string
}

type AppInfoBuilder interface {
	Build(f *frame.HandshakeFrame) (AppInfo, error)
}
