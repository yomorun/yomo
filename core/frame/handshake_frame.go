package frame

import (
	"github.com/yomorun/y3"
)

// HandshakeFrame is a Y3 encoded.
type HandshakeFrame struct {
	// Name is client name
	Name string
	// ClientID represents client ID
	ClientID string
	// ClientType represents client type (source or sfn)
	ClientType byte
	// ObserveDataTags are the client data tag list.
	ObserveDataTags []byte
	// auth
	authName    string
	authPayload string
}

// NewHandshakeFrame creates a new HandshakeFrame.
func NewHandshakeFrame(name string, clientID string, clientType byte, observeDataTags []byte, authName string, authPayload string) *HandshakeFrame {
	return &HandshakeFrame{
		Name:            name,
		ClientID:        clientID,
		ClientType:      clientType,
		ObserveDataTags: observeDataTags,
		authName:        authName,
		authPayload:     authPayload,
	}
}

// Type gets the type of Frame.
func (h *HandshakeFrame) Type() Type {
	return TagOfHandshakeFrame
}

// Encode to Y3 encoding.
func (h *HandshakeFrame) Encode() []byte {
	// name
	nameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeName))
	nameBlock.SetStringValue(h.Name)
	// client ID
	idBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeID))
	idBlock.SetStringValue(h.ClientID)
	// client type
	typeBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeType))
	typeBlock.SetBytesValue([]byte{h.ClientType})
	// observe data tags
	observeDataTagsBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeObserveDataTags))
	observeDataTagsBlock.SetBytesValue(h.ObserveDataTags)
	// auth
	authNameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeAuthName))
	authNameBlock.SetStringValue(h.authName)
	authPayloadBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeAuthPayload))
	authPayloadBlock.SetStringValue(h.authPayload)
	// handshake frame
	handshake := y3.NewNodePacketEncoder(byte(h.Type()))
	handshake.AddPrimitivePacket(nameBlock)
	handshake.AddPrimitivePacket(idBlock)
	handshake.AddPrimitivePacket(typeBlock)
	handshake.AddPrimitivePacket(observeDataTagsBlock)
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
	// name
	if nameBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.Name = name
	}
	// client ID
	if idBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.ClientID = id
	}
	// client type
	if typeBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeType)]; ok {
		clientType := typeBlock.ToBytes()
		handshake.ClientType = clientType[0]
	}
	// observe data tag list
	if observeDataTagsBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeObserveDataTags)]; ok {
		handshake.ObserveDataTags = observeDataTagsBlock.ToBytes()
	}
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
