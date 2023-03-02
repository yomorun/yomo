package frame

import "github.com/yomorun/y3"

// HandshakeAckFrame is used to ack handshake, It is always that the first frame
// is HandshakeAckFrame after client acquire a new stream.
// HandshakeAckFrame is a Y3 encoded bytes.
type HandshakeAckFrame struct{}

// NewHandshakeAckFrame returns a HandshakeAckFrame.
func NewHandshakeAckFrame() *HandshakeAckFrame {
	return &HandshakeAckFrame{}
}

// Type gets the type of the HandshakeAckFrame.
func (f *HandshakeAckFrame) Type() Type {
	return TagOfHandshakeAckFrame
}

// Encode encodes HandshakeAckFrame to Y3 encoded bytes.
func (f *HandshakeAckFrame) Encode() []byte {
	ack := y3.NewNodePacketEncoder(byte(f.Type()))

	return ack.Encode()
}

// DecodeToHandshakeAckFrame decodes Y3 encoded bytes to HandshakeAckFrame
func DecodeToHandshakeAckFrame(buf []byte) (*HandshakeAckFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	return &HandshakeAckFrame{}, nil
}
