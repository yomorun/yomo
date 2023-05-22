package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeBackflowFrame BackflowFrame to Y3 encoded bytes
func encodeBackflowFrame(f *frame.BackflowFrame) ([]byte, error) {
	tag := y3.NewPrimitivePacketEncoder(tagBackflowDataTag)
	tag.SetUInt32Value(uint32(f.Tag))

	carriage := y3.NewPrimitivePacketEncoder(tagBackflowCarriage)
	carriage.SetBytesValue(f.Carriage)

	node := y3.NewNodePacketEncoder(byte(f.Type()))
	node.AddPrimitivePacket(tag)
	node.AddPrimitivePacket(carriage)

	return node.Encode(), nil
}

// decodeBackflowFrame decodes Y3 encoded bytes to BackflowFrame.
func decodeBackflowFrame(data []byte, f *frame.BackflowFrame) error {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &nodeBlock)
	if err != nil {
		return err
	}

	if p, ok := nodeBlock.PrimitivePackets[tagBackflowDataTag]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = frame.Tag(tag)
	}

	if p, ok := nodeBlock.PrimitivePackets[tagBackflowCarriage]; ok {
		f.Carriage = p.GetValBuf()
	}

	return nil
}

var (
	tagBackflowDataTag  byte = 0x01
	tagBackflowCarriage byte = 0x02
)
