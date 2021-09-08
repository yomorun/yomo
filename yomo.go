package yomo

import (
	"github.com/yomorun/yomo/internal/frame"
)

// Metadata
type Metadata = frame.Metadata
type MetaFrame = frame.MetaFrame

var NewMetadata = frame.NewMetadata

// NewSource creates a new YoMo-Source client.
// func NewSource(opts ...Option) source.Client {
// 	options := newOptions(opts...)
// 	return source.New(options.AppName)
// }

// NewStreamFn creates a new YoMo-Stream-Function client.
// func NewStreamFn(opts ...Option) streamfunction.Client {
// 	options := newOptions(opts...)
// 	return streamfunction.New(options.AppName)
// }
