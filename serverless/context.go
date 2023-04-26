package serverless

// Context sfn handler context
type Context interface {
	// Data incoming data
	Data() []byte
	// Tag incoming tag
	Tag() uint32
	// Write write data to zipper
	Write(tag uint32, data []byte) error
}
