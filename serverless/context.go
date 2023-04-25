package serverless

import (
	"fmt"

	"github.com/yomorun/yomo/core/frame"
)

type Context struct {
	writer    frame.Writer
	dataFrame *frame.DataFrame
}

func NewContext(writer frame.Writer, dataFrame *frame.DataFrame) *Context {
	return &Context{
		writer:    writer,
		dataFrame: dataFrame,
	}
}

func (c *Context) Tag() uint32 {
	return c.dataFrame.Tag()
}

func (c *Context) Data() []byte {
	return c.dataFrame.GetCarriage()
}

func (c *Context) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	fmt.Printf("[serverless] write data with tag[%#v] to zipper: %s\n", tag, data)
	metaFrame := c.dataFrame.GetMetaFrame()
	dataFrame := frame.NewDataFrame()
	// reuse transactionID
	dataFrame.SetTransactionID(metaFrame.TransactionID())
	// reuse sourceID
	dataFrame.SetSourceID(metaFrame.SourceID())
	dataFrame.SetCarriage(tag, data)
	return c.writer.WriteFrame(dataFrame)
}
