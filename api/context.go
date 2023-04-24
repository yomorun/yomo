package api

var (
	// NewContext create a new context for handler
	NewContext func(tag uint32, data []byte) Context

	// DataTags set handler observed data tags
	DataTags func() []uint32 = func() []uint32 { return []uint32{0} }

	// Handler is the handler function for guest
	Handler func(ctx Context) = func(Context) {}
)

// Context handler context
type Context interface {
	// input
	Data() []byte
	// handler
	Tag() uint32
	Write(tag uint32, data []byte) error
}
