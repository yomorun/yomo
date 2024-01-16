// Package serverless provides the server serverless function context.
package serverless

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/payload"
)

// Context sfn handler context
type Context struct {
	writer frame.Writer
	tag    uint32
	md     metadata.M
	data   []byte
}

// NewContext creates a new serverless Context
func NewContext(writer frame.Writer, tag uint32, md metadata.M, data []byte) *Context {
	return &Context{
		writer: writer,
		tag:    tag,
		md:     md,
		data:   data,
	}
}

// Tag returns the tag of the data frame
func (c *Context) Tag() uint32 {
	return c.tag
}

// Data returns the data of the data frame
func (c *Context) Data() []byte {
	return c.data
}

// Write writes the data
func (c *Context) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}

	mdBytes, err := c.md.Encode()
	if err != nil {
		return err
	}

	dataFrame := &frame.DataFrame{
		Tag:      tag,
		Metadata: mdBytes,
		Payload:  data,
	}

	return c.writer.WriteFrame(dataFrame)
}

// WritePayload wirtes the payload.
func (c *Context) WritePayload(tag uint32, payload *payload.Payload) error {
	if payload == nil {
		return nil
	}
	if payload.Target != "" {
		c.md.Set(core.MetadataTargetKey, payload.Target)
	}
	if payload.TID != "" {
		c.md.Set(core.MetadataTIDKey, payload.TID)
	}
	mdBytes, err := c.md.Encode()
	if err != nil {
		return err
	}
	dataFrame := &frame.DataFrame{
		Tag:      tag,
		Metadata: mdBytes,
		Payload:  payload.Data,
	}

	return c.writer.WriteFrame(dataFrame)
}
