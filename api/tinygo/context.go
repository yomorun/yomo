//go:build tinygo || js || wasm

package tinygo

import (
	"fmt"

	"github.com/yomorun/yomo/api"
)

var _ api.Context = (*Context)(nil)

type Context struct {
	// input
	tag  uint32
	data []byte
}

func (c *Context) Tag() uint32 {
	return c.tag
}

func (c *Context) Data() []byte {
	return c.data
}

func (c *Context) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	fmt.Printf("input raw data with tag[%#v] to zipper: %s\n", c.tag, c.data)
	yomoDumpOutput(tag, &data[0], len(data))
	fmt.Printf("output data with tag[%#v] to zipper: %s\n", tag, data)
	return nil
}

func NewContext(tag uint32, data []byte) api.Context {
	return &Context{
		tag:  tag,
		data: data,
	}
}
