package frame

import (
	"github.com/yomorun/y3"
)

// HandshakeFrame is a Y3 encoded.
type HandshakeFrame struct {
	authName    string
	authPayload string
}

// NewHandshakeFrame creates a new HandshakeFrame.
func NewHandshakeFrame(authName string, authPayload string) *HandshakeFrame {
	return &HandshakeFrame{
		authName:    authName,
		authPayload: authPayload,
	}
}

// Type gets the type of Frame.
func (h *HandshakeFrame) Type() Type {
	return TagOfHandshakeFrame
}

// Encode to Y3 encoding.
func (h *HandshakeFrame) Encode() []byte {
	// auth
	authNameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeAuthName))
	authNameBlock.SetStringValue(h.authName)
	authPayloadBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeAuthPayload))
	authPayloadBlock.SetStringValue(h.authPayload)
	// handshake frame
	handshake := y3.NewNodePacketEncoder(byte(h.Type()))
	handshake.AddPrimitivePacket(authNameBlock)
	handshake.AddPrimitivePacket(authPayloadBlock)

	return handshake.Encode()
}

// DecodeToHandshakeFrame decodes Y3 encoded bytes to HandshakeFrame.
func DecodeToHandshakeFrame(buf []byte) (*HandshakeFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	handshake := &HandshakeFrame{}

	// auth
	if authNameBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeAuthName)]; ok {
		authName, err := authNameBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.authName = authName
	}
	if authPayloadBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeAuthPayload)]; ok {
		authPayload, err := authPayloadBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.authPayload = authPayload
	}

	return handshake, nil
}

// AuthPayload authentication payload
func (h *HandshakeFrame) AuthPayload() string {
	return h.authPayload
}

// AuthName authentication name
func (h *HandshakeFrame) AuthName() string {
	return h.authName
}
