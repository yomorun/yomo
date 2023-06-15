package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeObverseFrame encodes ObserveFrame to bytes in Y3 codec.
func encodeObverseFrame(f *frame.ObserveFrame) ([]byte, error) {
	// tag
	tagBlock := y3.NewPrimitivePacketEncoder(tagObverseTag)
	tagBlock.SetStringValue(f.Tag)

	// frame
	frame := y3.NewNodePacketEncoder(byte(f.Type()))
	frame.AddPrimitivePacket(tagBlock)

	return frame.Encode(), nil
}

// decodeObserveFrame decodes Y3 encoded bytes to ObserveFrame.
func decodeObserveFrame(data []byte, f *frame.ObserveFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	// tag
	if tagBlock, ok := node.PrimitivePackets[tagObverseTag]; ok {
		tag, err := tagBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	return nil
}

var (
	tagObverseTag byte = 0x01
)
