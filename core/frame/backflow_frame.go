package frame

import (
	"github.com/yomorun/y3"
)

// BackflowFrame is a Y3 encoded bytes
// It's used to receive stream function processed result
type BackflowFrame struct {
	Tag      byte
	Carriage []byte
}

// NewBackflowFrame creates a new BackflowFrame with a given tag and carriage
func NewBackflowFrame(tag byte, carriage []byte) *BackflowFrame {
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
	carriage := y3.NewPrimitivePacketEncoder(f.Tag)
	carriage.SetBytesValue(f.Carriage)

	node := y3.NewNodePacketEncoder(byte(TagOfBackflowFrame))
	node.AddPrimitivePacket(carriage)

	return node.Encode()
}

// GetDataTag return the Tag of user's data
func (f *BackflowFrame) GetDataTag() byte {
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
	for _, v := range nodeBlock.PrimitivePackets {
		payload.Tag = v.SeqID()
		payload.Carriage = v.GetValBuf()
		break
	}

	return payload, nil
}
