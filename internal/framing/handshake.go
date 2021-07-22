package framing

// HandshakeFrame represents a handshake frame.
type HandshakeFrame struct {
	*frame
}

// NewHandshakeFrame inits a new HandshakeFrame.
func NewHandshakeFrame(data []byte) *HandshakeFrame {
	return &HandshakeFrame{
		frame: newFrameWithData(FrameTypeHandshake, data),
	}
}
