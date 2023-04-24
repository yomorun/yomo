package serverless

import (
	"fmt"
	_ "unsafe"

	"github.com/yomorun/yomo/core/frame"
)

type HandlerContext struct {
	client    frame.Writer
	dataFrame *frame.DataFrame
}

func NewHandlerContext(client frame.Writer, dataFrame *frame.DataFrame) Context {
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
	dataFrame := frame.NewDataFrame()
	// reuse transactionID
	dataFrame.SetTransactionID(metaFrame.TransactionID())
	// reuse sourceID
	dataFrame.SetSourceID(metaFrame.SourceID())
	dataFrame.SetCarriage(tag, data)
	return hc.client.WriteFrame(dataFrame)
}

//export yomo_observe_datatag
//go:linkname yomoObserveDataTag
func yomoObserveDataTag(tag uint32)

//export yomo_load_input
//go:linkname yomoLoadInput
func yomoLoadInput(pointer *byte)

//export yomo_dump_output
//go:linkname yomoDumpOutput
func yomoDumpOutput(tag uint32, pointer *byte, length int)

//export yomo_write
// func yomoWrite(tag uint32, pointer *byte, length int)

//export yomo_init
//go:linkname yomoInit
func yomoInit() {
	dataTags := DataTags()
	for _, tag := range dataTags {
		yomoObserveDataTag(uint32(tag))
	}
}

//export yomo_handler
//go:linkname yomoHandler
func yomoHandler(inputLength int) {
	// load input data
	input := make([]byte, inputLength)
	yomoLoadInput(&input[0])
	// handler
	// ctx := api.NewContext(0x33, input)
	ctx := NewContext(nil, nil)
	if ctx == nil {
		return
	}
	// Handler(ctx)
	Handler(input)
}
