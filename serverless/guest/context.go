// guest wasm application programming interface for guest module
package guest

import (
	_ "unsafe"

	"github.com/yomorun/yomo/serverless"
)

var (

	// DataTags set handler observed data tags
	DataTags func() []uint32 = func() []uint32 { return []uint32{0} }

	// Handler is the handler function for guest
	Handler func(ctx serverless.Context) = func(serverless.Context) {}
	// Handler func(ctx Context, input []byte)
)

type GuestContext struct{}

func (c *GuestContext) Tag() uint32 {
	return yomoContextTag()
}

func (c *GuestContext) Data() []byte {
	return GetBytes(ContextData)
}

func (c *GuestContext) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	yomoWrite(tag, &data[0], len(data))
	return nil
}

//export yomo_observe_datatag
//go:linkname yomoObserveDataTag
func yomoObserveDataTag(tag uint32)

//export yomo_write
//go:linkname yomoWrite
func yomoWrite(tag uint32, pointer *byte, length int)

//export yomo_context_tag
//go:linkname yomoContextTag
func yomoContextTag() uint32

//export yomo_context_data
//go:linkname contextData
func contextData(ptr uintptr, size uint32) uint32

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
func yomoHandler() {
	ctx := &GuestContext{}
	Handler(ctx)
}

func ContextData(ptr uintptr, size uint32) uint32 {
	return contextData(ptr, size)
}
