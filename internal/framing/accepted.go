package framing

// AcceptedFrame represents an Accepected frame.
type AcceptedFrame struct {
	*frame
}

// NewAcceptedFrame inits a new AcceptedFrame.
func NewAcceptedFrame() *AcceptedFrame {
	return &AcceptedFrame{
		frame: newFrame(FrameTypeAccepted),
	}
}
