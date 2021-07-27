package framing

// CreateStreamFrame represents a CREATE_STREAM frame.
type CreateStreamFrame struct {
	*frame
}

// NewCreateStreamFrame inits a new CREATE_STREAM frame.
func NewCreateStreamFrame(opts ...Option) *CreateStreamFrame {
	return &CreateStreamFrame{
		frame: newFrame(FrameTypeCreateStream, opts...),
	}
}
