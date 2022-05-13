package core

import "github.com/yomorun/yomo/core/frame"

// AppInfo is used for customizing extensions of an application.
type AppInfo interface {
	// Key must be globally unique between applications
	Key() string
}

// AppInfoBuilder is the builder for AppInfo
type AppInfoBuilder interface {
	// Build will return an AppInfo instance according to the frame passed in
	Build(f frame.Frame) (AppInfo, error)
}
