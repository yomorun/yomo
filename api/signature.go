// // wasm API
package api

// import "github.com/yomorun/yomo/core/frame"

// var (
// 	// NewContext create a new context for handler
// 	// NewContext func(tag uint32, data []byte) Context
// 	NewContext func(frame.Writer, *frame.DataFrame) Context

// 	// DataTags set handler observed data tags
// 	DataTags func() []uint32 = func() []uint32 { return []uint32{0} }

// 	// Handler is the handler function for guest
// 	Handler func(ctx Context) = func(Context) {}
// 	// Handler func(ctx Context, input []byte)
// )

// // Context handler context
// type Context interface {
// 	// input
// 	Data() []byte
// 	DataFrame() *frame.DataFrame
// 	Writer() frame.Writer
// 	// handler
// 	Tag() uint32
// 	Write(tag uint32, data []byte) error
// }
