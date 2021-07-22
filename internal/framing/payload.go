package framing

// PayloadFrame represents a payload frame.
type PayloadFrame struct {
	*frame
}

// NewPayloadFrame inits a new PayloadFrame.
func NewPayloadFrame(data []byte) *PayloadFrame {
	return &PayloadFrame{
		frame: newFrameWithData(FrameTypePayload, data),
	}
}
