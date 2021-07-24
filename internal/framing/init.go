package framing

// InitFrame represents an Accepected frame.
type InitFrame struct {
	*frame
}

// NewInitFrame inits a new InitFrame.
func NewInitFrame() *InitFrame {
	return &InitFrame{
		frame: newFrame(FrameTypeInit),
	}
}
