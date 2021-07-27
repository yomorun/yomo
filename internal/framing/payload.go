package framing

// PayloadFrame represents a PAYLOAD frame.
type PayloadFrame struct {
	*frame
}

// NewPayloadFrame inits a new PAYLOAD frame.
func NewPayloadFrame(data []byte, opts ...Option) *PayloadFrame {
	return &PayloadFrame{
		frame: newFrameWithData(FrameTypePayload, data, opts...),
	}
}
