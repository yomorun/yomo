package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeRejectedFrame encodes RejectedFrame to Y3 encoded bytes.
func encodeRejectedFrame(f *frame.RejectedFrame) ([]byte, error) {
	// code
	codeBlock := y3.NewPrimitivePacketEncoder(tagRejectedCode)
	codeBlock.SetUInt64Value(f.Code)
	// message
	messageBlock := y3.NewPrimitivePacketEncoder(byte(tagRejectedMessage))
	messageBlock.SetStringValue(f.Message)
	// frame
	ff := y3.NewNodePacketEncoder(byte(f.Type()))
	ff.AddPrimitivePacket(codeBlock)
	ff.AddPrimitivePacket(messageBlock)

	return ff.Encode(), nil
}

// decodeRejectedFrame decodes Y3 encoded bytes to RejectedFrame.
func decodeRejectedFrame(data []byte, f *frame.RejectedFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// code
	if codeBlock, ok := node.PrimitivePackets[tagRejectedCode]; ok {
		code, err := codeBlock.ToUInt64()
		if err != nil {
			return err
		}
		f.Code = code
	}
	// message
	if messageBlock, ok := node.PrimitivePackets[tagRejectedMessage]; ok {
		message, err := messageBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Message = message
	}

	return nil
}

var (
	tagRejectedCode    byte = 0x01
	tagRejectedMessage byte = 0x02
)
