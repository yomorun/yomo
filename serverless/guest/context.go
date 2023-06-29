// guest wasm application programming interface for guest module
package guest

import (
	"errors"
	_ "unsafe"

	"github.com/yomorun/yomo/serverless"
)

var (
	// DataTags set handler observed data tags
	DataTags func() []uint32 = func() []uint32 { return []uint32{0} }
	// Handler is the handler function for guest
	Handler func(ctx serverless.Context) = func(serverless.Context) {}
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
	if yomoWrite(tag, &data[0], len(data)) != 0 {
		return errors.New("yomoWrite error")
	}
	return nil
}

//export yomo_observe_datatag
//go:linkname yomoObserveDataTag
func yomoObserveDataTag(tag uint32)

//export yomo_write
//go:linkname yomoWrite
func yomoWrite(tag uint32, pointer *byte, length int) uint32

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
		yomoObserveDataTag(tag)
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
