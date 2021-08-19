package yomo

import (
	"github.com/yomorun/yomo/connector/output"
	"github.com/yomorun/yomo/source"
	"github.com/yomorun/yomo/streamfunction"
)

// NewSource creates a new YoMo-Source client.
func NewSource(opts ...Option) source.Client {
	options := newOptions(opts...)
	return source.New(options.AppName)
}

// NewStreamFn creates a new YoMo-Stream-Function client.
func NewStreamFn(opts ...Option) streamfunction.Client {
	options := newOptions(opts...)
	return streamfunction.New(options.AppName)
}

// NewOutputConnector creates a new YoMo-Stream-Function client.
func NewOutputConnector(opts ...Option) output.Client {
	options := newOptions(opts...)
	return output.New(options.AppName)
}
