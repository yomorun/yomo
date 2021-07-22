package framing

// RejectedFrame represents an Accepected frame.
type RejectedFrame struct {
	*frame
}

// NewRejectedFrame inits a new RejectedFrame.
func NewRejectedFrame() *RejectedFrame {
	return &RejectedFrame{
		frame: newFrame(FrameTypeRejected),
	}
}
