package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeGoawayFrame encodes GoawayFrame to Y3 encoded bytes.
func encodeGoawayFrame(f *frame.GoawayFrame) ([]byte, error) {
	// message
	messageBlock := y3.NewPrimitivePacketEncoder(byte(tagGoawayMessage))
	messageBlock.SetStringValue(f.Message)
	// frame
	ff := y3.NewNodePacketEncoder(byte(f.Type()))
	ff.AddPrimitivePacket(messageBlock)

	return ff.Encode(), nil
}

// decodeGoawayFrame decodes Y3 encoded bytes to GoawayFrame.
func decodeGoawayFrame(data []byte, f *frame.GoawayFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// message
	if messageBlock, ok := node.PrimitivePackets[tagGoawayMessage]; ok {
		message, err := messageBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Message = message
	}

	return nil
}

var (
	tagGoawayMessage byte = 0x01
)
