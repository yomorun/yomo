package serverless

import (
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// CronContext sfn cron handler context
type CronContext struct {
	writer  frame.Writer
	md      metadata.M
	mdBytes []byte
}

// NewCronContext creates a new serverless CronContext
func NewCronContext(writer frame.Writer, md metadata.M) *CronContext {
	mdBytes, _ := md.Encode()

	return &CronContext{
		writer:  writer,
		md:      md,
		mdBytes: mdBytes,
	}
}

// Write writes the data to next sfn instance.
func (c *CronContext) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}

	dataFrame := &frame.DataFrame{
		Tag:      tag,
		Metadata: c.mdBytes,
		Payload:  data,
	}

	return c.writer.WriteFrame(dataFrame)
}

// WriteWithTarget writes the data to next sfn instance with specified target.
func (c *CronContext) WriteWithTarget(tag uint32, data []byte, target string) error {
	if data == nil {
		return nil
	}

	if target != "" {
		c.md.Set(metadata.TargetKey, target)
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
