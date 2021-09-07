package yomo

import (
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/streamfunction"
)

// Metadata
type Metadata = frame.Metadata

var NewMetadata = frame.NewMetadata

// NewSource creates a new YoMo-Source client.
// func NewSource(opts ...Option) source.Client {
// 	options := newOptions(opts...)
// 	return source.New(options.AppName)
// }

// NewStreamFn creates a new YoMo-Stream-Function client.
func NewStreamFn(opts ...Option) streamfunction.Client {
	options := newOptions(opts...)
	return streamfunction.New(options.AppName)
}
