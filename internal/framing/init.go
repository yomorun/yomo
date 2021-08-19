package framing

// InitFrame represents an INIT frame.
type InitFrame struct {
	*frame
}

// NewInitFrame inits a new INIT frame.
func NewInitFrame(opts ...Option) *InitFrame {
	return &InitFrame{
		frame: newFrame(FrameTypeInit, opts...),
	}
}
