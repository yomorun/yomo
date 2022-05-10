package frame

// ExtFrame is used for further extensions of the MetaFrame
type ExtFrame interface {
	// Encode the frame into []byte.
	Encode() []byte
}

// ExtFrameBuilder is the factory manager for ExtFrame
type ExtFrameBuilder interface {
	// NewExtFrame creates a new ExtFrame instance.
	NewExtFrame() ExtFrame
	// DecodeToExtFrame decodes a ExtFrame instance from given buffer.
	DecodeToExtFrame(buf []byte) (ExtFrame, error)
}

// RegisterExtFrameBuilder is used for developers to implement their own ExtFrameBuilder
func RegisterExtFrameBuilder(builder ExtFrameBuilder) {
	extFrameBuilder = builder
}

var extFrameBuilder ExtFrameBuilder
