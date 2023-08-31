package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeGoawayFrame encodes GoawayFrame to Y3 encoded bytes.
func encodeGoawayFrame(f *frame.GoawayFrame) ([]byte, error) {
	// code
	codeBlock := y3.NewPrimitivePacketEncoder(tagGoawayCode)
	codeBlock.SetUInt64Value(f.Code)
	// message
	messageBlock := y3.NewPrimitivePacketEncoder(byte(tagGoawayMessage))
	messageBlock.SetStringValue(f.Message)
	// frame
	ff := y3.NewNodePacketEncoder(byte(f.Type()))
	ff.AddPrimitivePacket(codeBlock)
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

	// code
	if codeBlock, ok := node.PrimitivePackets[tagGoawayCode]; ok {
		code, err := codeBlock.ToUInt64()
		if err != nil {
			return err
		}
		f.Code = code
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
	tagGoawayCode    byte = 0x01
	tagGoawayMessage byte = 0x02
)
