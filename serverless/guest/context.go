// Package guest wasm application programming interface for guest module
package guest

import (
	"errors"
	_ "unsafe"

	"github.com/yomorun/yomo/serverless"
)

var (
	// DataTags sets handler observed data tags
	DataTags func() []uint32 = func() []uint32 { return []uint32{0} }
	// WantedTarget sets handler wanted target
	WantedTarget func() string = func() string { return "" }
	// Handler is the handler function for guest
	Handler func(ctx serverless.Context) = func(serverless.Context) {}
	// Init is the init function for guest
	Init func() error = func() error { return nil }
)

var _ serverless.Context = (*GuestContext)(nil)

// GuestContext is the context for guest
type GuestContext struct{}

// Tag returns the tag of the context
func (c *GuestContext) Tag() uint32 {
	return yomoContextTag()
}

// Data returns the data of the context
func (c *GuestContext) Data() []byte {
	return GetBytes(ContextData)
}

// Metadata returns the value of from metadata in key
func (c *GuestContext) Metadata(key string) (string, bool) {
	panic("not implemented")
}

// Write writes data to the context
func (c *GuestContext) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}
	if yomoWrite(tag, &data[0], len(data)) != 0 {
		return errors.New("yomoWrite error")
	}
	return nil
}

// WriteWithTarget writes data with target to the context
func (c *GuestContext) WriteWithTarget(tag uint32, data []byte, target string) error {
	if data == nil {
		return nil
	}
	if target == "" {
		return c.Write(tag, data)
	}
	targetBytes := []byte(target)
	if yomoWriteWithTarget(tag, &data[0], len(data), &targetBytes[0], len(targetBytes)) != 0 {
		return errors.New("yomoWriteWithTarget error")
	}
	return nil
}

//export yomo_observe_datatag
//go:linkname yomoObserveDataTag
func yomoObserveDataTag(tag uint32)

//export yomo_write
//go:linkname yomoWrite
func yomoWrite(tag uint32, pointer *byte, length int) uint32

//export yomo_write_with_target
//go:linkname yomoWriteWithTarget
func yomoWriteWithTarget(tag uint32, pointer *byte, length int, targetPointer *byte, targetLength int) uint32

//export yomo_context_tag
//go:linkname yomoContextTag
func yomoContextTag() uint32

//export yomo_context_data
//go:linkname contextData
func contextData(ptr uintptr, size uint32) uint32

//export yomo_observe_datatags
//go:linkname yomoObserveDataTags
func yomoObserveDataTags() {
	// set observe data tags
	dataTags := DataTags()
	for _, tag := range dataTags {
		yomoObserveDataTag(tag)
	}
}

//export yomo_get_wanted_target
//go:linkname yomoGetWantedTarget
func yomoGetWantedTarget(ptr uintptr, size uint32)

//export yomo_wanted_target
//go:linkname yomoWantedTarget
func yomoWantedTarget() {
	target := WantedTarget()
	if target == "" {
		return
	}
	yomoGetWantedTarget(bufferToPtrSize([]byte(target)))
}

//export yomo_handler
//go:linkname yomoHandler
func yomoHandler() {
	ctx := &GuestContext{}
	Handler(ctx)
}

//export yomo_init
//go:linkname yomoInit
func yomoInit() uint32 {
	// init
	if err := Init(); err != nil {
		print("yomoInit error: ", err)
		return 1
	}
	return 0
}

// ContextData returns the data of the context
func ContextData(ptr uintptr, size uint32) uint32 {
	return contextData(ptr, size)
}
