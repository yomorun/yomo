package framing

// AckFrame represents an ACK frame.
type AckFrame struct {
	*frame
}

// NewAckFrame inits a new ACK frame.
func NewAckFrame(opts ...Option) *AckFrame {
	return &AckFrame{
		frame: newFrame(FrameTypeAck, opts...),
	}
}
