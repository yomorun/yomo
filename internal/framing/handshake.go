package framing

// HandshakeFrame represents a HANDSHAKE frame.
type HandshakeFrame struct {
	*frame
}

// NewHandshakeFrame inits a new HANDSHAKE frame.
func NewHandshakeFrame(data []byte, opts ...Option) *HandshakeFrame {
	return &HandshakeFrame{
		frame: newFrameWithData(FrameTypeHandshake, data, opts...),
	}
}
