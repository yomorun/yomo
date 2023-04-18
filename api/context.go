package api

import (
	"fmt"
)

//export yomo_dump_output
func yomoDumpOutput(tag uint32, pointer *byte, length int)

type Request struct {
	Tags []Tag
	Data []byte
}

type Context interface {
	// request
	DataTags() []Tag
	Data() []byte
	// response
	Write(tag Tag, data []byte) error
}

type DefaultContext struct {
	dataTags []Tag
	data     []byte
}

func (c *DefaultContext) DataTags() []Tag {
	return c.dataTags
}

func (c *DefaultContext) Data() []byte {
	return c.data
}

func (c *DefaultContext) Write(tag Tag, data []byte) error {
	// tag, output := Handler(ctx)
	// dump output data
	// if output == nil {
	// 	return
	// }
	// yomoDumpOutput(uint32(tag), &output[0], len(output))
	fmt.Printf("write data with tag[%#v] to zipper: %s\n", tag, data)
	yomoDumpOutput(uint32(tag), &data[0], len(data))
	return nil
}

func NewContext(tags []Tag, data []byte) *DefaultContext {
	return &DefaultContext{
		dataTags: tags,
		data:     data,
	}
}
