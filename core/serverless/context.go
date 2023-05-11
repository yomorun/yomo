package serverless

import "github.com/yomorun/yomo/core/frame"

// Context sfn handler context
type Context struct {
	writer    frame.Writer
	dataFrame *frame.DataFrame
}

// NewContext creates a new serverless Context
func NewContext(writer frame.Writer, dataFrame *frame.DataFrame) *Context {
	return &Context{
		writer:    writer,
		dataFrame: dataFrame,
	}
}

// Tag returns the tag of the data frame
func (c *Context) Tag() uint32 {
	return c.dataFrame.Tag()
}

// Data returns the data of the data frame
func (c *Context) Data() []byte {
	return c.dataFrame.GetCarriage()
}

// Write writes the data
func (c *Context) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	metaFrame := c.dataFrame.GetMetaFrame()
	dataFrame := frame.NewDataFrame()
	// reuse transactionID
	dataFrame.SetTransactionID(metaFrame.TransactionID())
	// reuse sourceID
	dataFrame.SetSourceID(metaFrame.SourceID())
	dataFrame.SetCarriage(tag, data)
	return c.writer.WriteFrame(dataFrame)
}
