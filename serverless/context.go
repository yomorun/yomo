package serverless

// Context sfn handler context
type Context interface {
	// input
	Data() []byte
	// handler
	Tag() uint32
	Write(tag uint32, data []byte) error
}
