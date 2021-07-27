package framing

// RejectedFrame represents a REJECTED frame.
type RejectedFrame struct {
	*frame
}

// NewRejectedFrame inits a new REJECTED frame.
func NewRejectedFrame(opts ...Option) *RejectedFrame {
	return &RejectedFrame{
		frame: newFrame(FrameTypeRejected, opts...),
	}
}
