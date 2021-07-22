package framing

// CreateStreamFrame represents an Accepected frame.
type CreateStreamFrame struct {
	*frame
}

// NewCreateStreamFrame inits a new CreateStreamFrame.
func NewCreateStreamFrame() *CreateStreamFrame {
	return &CreateStreamFrame{
		frame: newFrame(FrameTypeCreateStream),
	}
}
