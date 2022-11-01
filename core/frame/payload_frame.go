package frame

import (
	"github.com/yomorun/y3"
)

// PayloadFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_PAYLOAD_FRAME
// the Len is the length of Val. Val is also a Y3 encoded PrimitivePacket, storing
// raw bytes as user's data
type PayloadFrame struct {
	Tag      uint32
	Carriage []byte
}

// NewPayloadFrame creates a new PayloadFrame with a given TagID of user's data
func NewPayloadFrame(tag uint32) *PayloadFrame {
	return &PayloadFrame{
		Tag: tag,
	}
}

// SetCarriage sets the user's raw data
func (m *PayloadFrame) SetCarriage(buf []byte) *PayloadFrame {
	m.Carriage = buf
	return m
}

// Encode to Y3 encoded bytes
func (m *PayloadFrame) Encode() []byte {
	tag := y3.NewPrimitivePacketEncoder(byte(TagOfPayloadDataTag))
	tag.SetUInt32Value(m.Tag)

	carriage := y3.NewPrimitivePacketEncoder(byte(TagOfPayloadCarriage))
	carriage.SetBytesValue(m.Carriage)

	payload := y3.NewNodePacketEncoder(byte(TagOfPayloadFrame))
	payload.AddPrimitivePacket(tag)
	payload.AddPrimitivePacket(carriage)

	return payload.Encode()
}

// DecodeToPayloadFrame decodes Y3 encoded bytes to PayloadFrame
func DecodeToPayloadFrame(buf []byte) (*PayloadFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}

	payload := &PayloadFrame{}
	if p, ok := nodeBlock.PrimitivePackets[byte(TagOfPayloadDataTag)]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return nil, err
		}
		payload.Tag = tag
	}

	if p, ok := nodeBlock.PrimitivePackets[byte(TagOfPayloadCarriage)]; ok {
		payload.Carriage = p.GetValBuf()
	}

	return payload, nil
}
