package framing

// AcceptedFrame represents an ACCEPTED frame.
type AcceptedFrame struct {
	*frame
}

// NewAcceptedFrame inits a new ACCEPTED frame.
func NewAcceptedFrame(opts ...Option) *AcceptedFrame {
	return &AcceptedFrame{
		frame: newFrame(FrameTypeAccepted, opts...),
	}
}
