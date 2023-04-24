package core

import (
	"fmt"

	"github.com/yomorun/yomo/core/frame"
)

// AsyncHandler is the request-response mode (asnyc)
// type AsyncHandler func(tag uint32, data []byte) (uint32, []byte)
type AsyncHandler func(hctx *HandlerContext)

// PipeHandler is the bidirectional stream mode (blocking).
type PipeHandler func(in <-chan []byte, out chan<- *frame.PayloadFrame)

type HandlerContext struct {
	client    *Client
	dataFrame *frame.DataFrame
}

func NewHandlerContext(client *Client, dataFrame *frame.DataFrame) *HandlerContext {
	return &HandlerContext{
		client:    client,
		dataFrame: dataFrame,
	}
}

func (hc *HandlerContext) Tag() uint32 {
	return hc.dataFrame.Tag()
}

func (hc *HandlerContext) Data() []byte {
	return hc.dataFrame.GetCarriage()
}

func (hc *HandlerContext) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	fmt.Printf("write data with tag[%#v] to zipper: %s\n", tag, data)
	metaFrame := hc.dataFrame.GetMetaFrame()
	frame := frame.NewDataFrame()
	// reuse transactionID
	frame.SetTransactionID(metaFrame.TransactionID())
	// reuse sourceID
	frame.SetSourceID(metaFrame.SourceID())
	frame.SetCarriage(tag, data)
	return hc.client.WriteFrame(frame)
}
