package core

import "github.com/yomorun/yomo/core/frame"

// AppInfo is used for customizing extensions of an application.
type AppInfo interface {
	// Key must be globally unique between applications
	Key() string
	// Encode is the serializer method
	Encode() []byte
}

// AppInfoBuilder is the builder for AppInfo
type AppInfoBuilder interface {
	// Build will return an AppInfo instance according to the frame passed in
	Build(f *frame.HandshakeFrame) (AppInfo, error)
	// Decode is the deserializer method
	Decode(buf []byte) (AppInfo, error)
}
