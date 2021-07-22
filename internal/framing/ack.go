package framing

// AckFrame represents an ACK frame.
type AckFrame struct {
	*frame
}

// NewAckFrame inits a new AckFrame.
func NewAckFrame() *AckFrame {
	return &AckFrame{
		frame: newFrame(FrameTypeAck),
	}
}
