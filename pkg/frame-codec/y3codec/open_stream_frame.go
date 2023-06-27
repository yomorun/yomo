package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeOpenStreamFrame encodes OpenStreamFrame to Y3 encoded bytes.
func encodeOpenStreamFrame(f *frame.OpenStreamFrame) ([]byte, error) {
	// id
	idBlock := y3.NewPrimitivePacketEncoder(tagOpenStreamID)
	idBlock.SetStringValue(f.ID)
	// tag
	tagBlock := y3.NewPrimitivePacketEncoder(tagOpenStreamTag)
	tagBlock.SetStringValue(f.Tag)
	// frame
	ff := y3.NewNodePacketEncoder(byte(f.Type()))
	ff.AddPrimitivePacket(idBlock)
	ff.AddPrimitivePacket(tagBlock)

	return ff.Encode(), nil
}

// decodeOpenStreamFrame decodes Y3 encoded bytes to OpenStreamFrame.
func decodeOpenStreamFrame(data []byte, f *frame.OpenStreamFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// id
	if idBlock, ok := node.PrimitivePackets[tagOpenStreamID]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.ID = id
	}
	// tag
	if tagBlock, ok := node.PrimitivePackets[tagOpenStreamTag]; ok {
		tag, err := tagBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	return nil
}

var (
	tagOpenStreamID  byte = 0x01
	tagOpenStreamTag byte = 0x02
)
