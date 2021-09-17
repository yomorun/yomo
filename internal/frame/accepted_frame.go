package frame

import "github.com/yomorun/y3"

// AcceptedFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_ACCEPTED_FRAME
type AcceptedFrame struct{}

// NewAcceptedFrame creates a new AcceptedFrame with a given TagID of user's data
func NewAcceptedFrame() *AcceptedFrame {
	return &AcceptedFrame{}
}

// Type gets the type of Frame.
func (m *AcceptedFrame) Type() Type {
	return TagOfAcceptedFrame
}

// Encode to Y3 encoded bytes.
func (m *AcceptedFrame) Encode() []byte {
	accepted := y3.NewNodePacketEncoder(byte(m.Type()))
	accepted.AddBytes(nil)

	return accepted.Encode()
}

// DecodeToAcceptedFrame decodes Y3 encoded bytes to AcceptedFrame.
func DecodeToAcceptedFrame(buf []byte) (*AcceptedFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}
	return &AcceptedFrame{}, nil
}
