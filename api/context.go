// wasm API
package api

import (
	"fmt"
	"reflect"
	"unsafe"
	_ "unsafe"
)

var (

	// DataTags set handler observed data tags
	DataTags func() []uint32 = func() []uint32 { return []uint32{0} }

	// Handler is the handler function for guest
	Handler func(ctx *Context) = func(*Context) {}
	// Handler func(ctx Context, input []byte)
)

type Context struct{}

func NewContext() *Context {
	return &Context{}
}

func (c *Context) Tag() uint32 {
	return yomoContextTag()
}

func (c *Context) Data() []byte {
	// n := rand.Intn(100)
	// return []byte(fmt.Sprintf("source data:%d", n))
	// TODO: get from host
	ptrSize := yomoContextData()
	ptr, size := UnpackUint32(ptrSize)
	// buf:=make([]byte,size)
	// return input
	// data := ptrToString(ptr, size)
	data := ptrToBytes(ptr, size)
	fmt.Printf("[WasmContext] received data: prt=%v, size=%v, data=%s\n", ptr, size, data)
	return []byte(data)
}

func ptrToString(ptr uint32, size uint32) string {
	// Get a slice view of the underlying bytes in the stream. We use SliceHeader, not StringHeader
	// as it allows us to fix the capacity to what was allocated.
	return *(*string)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(ptr),
		Len:  uintptr(size), // Tinygo requires these as uintptrs even if they are int fields.
		Cap:  uintptr(size), // ^^ See https://github.com/tinygo-org/tinygo/issues/1284
	}))
}

func ptrToBytes(ptr uint32, size uint32) []byte {
	// Get a slice view of the underlying bytes in the stream. We use SliceHeader, not StringHeader
	// as it allows us to fix the capacity to what was allocated.
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(ptr),
		Len:  uintptr(size), // Tinygo requires these as uintptrs even if they are int fields.
		Cap:  uintptr(size), // ^^ See https://github.com/tinygo-org/tinygo/issues/1284
	}))
}

// func (c *Context) DataFrame() *frame.DataFrame {
// 	return c.dataFrame
// }

// func (c *Context) Writer() frame.Writer {
// 	return c.client
// }

func (c *Context) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	fmt.Printf("[WasmContext] write data with tag[%#v] to zipper: %s\n", tag, data)
	yomoWrite(tag, &data[0], len(data))
	return nil
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
//go:linkname yomoWrite
func yomoWrite(tag uint32, pointer *byte, length int)

//export yomo_context_tag
//go:linkname yomoContextTag
func yomoContextTag() uint32

//export yomo_context_data
//go:linkname yomoContextData
func yomoContextData() uint64

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
	// input := make([]byte, inputLength)
	// yomoLoadInput(&input[0])
	// handler
	// ctx := api.NewContext(0x33, input)
	// ctx := NewContext(nil, nil)
	// if ctx == nil {
	// 	return
	// }
	ctx := &Context{}
	Handler(ctx)
	// Handler(input)
}

func UnpackUint32(packed uint64) (uint32, uint32) {
	return uint32(packed >> 32), uint32(packed)
}
