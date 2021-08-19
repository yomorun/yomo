package frame

import (
	"github.com/yomorun/y3"
)

// HandshakeFrame is a Y3 encoded.
type HandshakeFrame struct {
	// Name is client name
	Name string
	// ClientType represents client type (source or sfn)
	ClientType string
}

// NewHandshakeFrame creates a new HandshakeFrame.
func NewHandshakeFrame(name, clientType string) *HandshakeFrame {
	return &HandshakeFrame{
		Name:       name,
		ClientType: clientType,
	}
}

// Type gets the type of Frame.
func (h *HandshakeFrame) Type() FrameType {
	return TagOfHandshakeFrame
}

// Encode to Y3 encoding.
func (h *HandshakeFrame) Encode() []byte {
	nameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeName))
	nameBlock.SetStringValue(h.Name)

	typeBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeType))
	typeBlock.SetStringValue(h.ClientType)

	handshake := y3.NewNodePacketEncoder(byte(h.Type()))
	handshake.AddPrimitivePacket(nameBlock)
	handshake.AddPrimitivePacket(typeBlock)

	return handshake.Encode()
}

// DecodeToHandshakeFrame return a HandshakeFrame from buffer
func DecodeToHandshakeFrame(buf []byte) (*HandshakeFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	handshake := &HandshakeFrame{}

	if nameBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.Name = name
	}

	if typeBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeType)]; ok {
		clientType, err := typeBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.ClientType = clientType
	}

	return handshake, nil
}
