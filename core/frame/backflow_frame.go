package frame

import (
	"github.com/yomorun/y3"
)

// BackflowFrame is a Y3 encoded bytes
// It's used to receive stream function processed result
type BackflowFrame struct {
	Tag      uint32
	Carriage []byte
}

// NewBackflowFrame creates a new BackflowFrame with a given tag and carriage
func NewBackflowFrame(tag uint32, carriage []byte) *BackflowFrame {
	return &BackflowFrame{
		Tag:      tag,
		Carriage: carriage,
	}
}

// Type gets the type of Frame.
func (f *BackflowFrame) Type() Type {
	return TagOfBackflowFrame
}

// SetCarriage sets the user's raw data
func (f *BackflowFrame) SetCarriage(buf []byte) *BackflowFrame {
	f.Carriage = buf
	return f
}

// Encode to Y3 encoded bytes
func (f *BackflowFrame) Encode() []byte {
	tag := y3.NewPrimitivePacketEncoder(byte(TagOfBackflowDataTag))
	tag.SetUInt32Value(f.Tag)

	carriage := y3.NewPrimitivePacketEncoder(byte(TagOfBackflowCarriage))
	carriage.SetBytesValue(f.Carriage)

	node := y3.NewNodePacketEncoder(byte(TagOfBackflowFrame))
	node.AddPrimitivePacket(tag)
	node.AddPrimitivePacket(carriage)
	return node.Encode()
}

// GetDataTag return the Tag of user's data
func (f *BackflowFrame) GetDataTag() uint32 {
	return f.Tag
}

// GetCarriage return data
func (f *BackflowFrame) GetCarriage() []byte {
	return f.Carriage
}

// DecodeToBackflowFrame decodes Y3 encoded bytes to BackflowFrame
func DecodeToBackflowFrame(buf []byte) (*BackflowFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}

	payload := &BackflowFrame{}
	if p, ok := nodeBlock.PrimitivePackets[byte(TagOfBackflowDataTag)]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return nil, err
		}
		payload.Tag = tag
	}

	if p, ok := nodeBlock.PrimitivePackets[byte(TagOfBackflowCarriage)]; ok {
		payload.Carriage = p.GetValBuf()
	}

	return payload, nil
}
