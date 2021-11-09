package frame

import "github.com/yomorun/y3"

// RejectedFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_REJECTED_FRAME
type RejectedFrame struct{}

// NewRejectedFrame creates a new RejectedFrame with a given TagID of user's data
func NewRejectedFrame() *RejectedFrame {
	return &RejectedFrame{}
}

// Type gets the type of Frame.
func (m *RejectedFrame) Type() Type {
	return TagOfRejectedFrame
}

// Encode to Y3 encoded bytes
func (m *RejectedFrame) Encode() []byte {
	rejected := y3.NewNodePacketEncoder(byte(m.Type()))
	rejected.AddBytes(nil)

	return rejected.Encode()
}

// DecodeToRejectedFrame decodes Y3 encoded bytes to RejectedFrame
func DecodeToRejectedFrame(buf []byte) (*RejectedFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}
	return &RejectedFrame{}, nil
}
