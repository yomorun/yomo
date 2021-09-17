package frame

import (
	"github.com/yomorun/y3"
)

// PingFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_PING_FRAME
type PingFrame struct{}

// NewPingFrame creates a new PingFrame with a given TagID of user's data
func NewPingFrame() *PingFrame {
	return &PingFrame{}
}

// Type gets the type of Frame.
func (m *PingFrame) Type() Type {
	return TagOfPingFrame
}

// Encode to Y3 encoded bytes
func (m *PingFrame) Encode() []byte {
	ping := y3.NewNodePacketEncoder(byte(m.Type()))
	ping.AddBytes(nil)

	return ping.Encode()
}

// DecodeToPingFrame decodes Y3 encoded bytes to PingFrame
func DecodeToPingFrame(buf []byte) (*PingFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}
	return &PingFrame{}, nil
}

// PongFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_PONG_FRAME
type PongFrame struct{}

// NewPongFrame creates a new PongFrame with a given TagID of user's data
func NewPongFrame() *PongFrame {
	return &PongFrame{}
}

// Type gets the type of Frame.
func (m *PongFrame) Type() Type {
	return TagOfPongFrame
}

// Encode to Y3 encoded bytes
func (m *PongFrame) Encode() []byte {
	pong := y3.NewNodePacketEncoder(byte(m.Type()))
	pong.AddBytes(nil)

	return pong.Encode()
}

// DecodeToPongFrame decodes Y3 encoded bytes to PongFrame
func DecodeToPongFrame(buf []byte) (*PongFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}
	return &PongFrame{}, nil
}
