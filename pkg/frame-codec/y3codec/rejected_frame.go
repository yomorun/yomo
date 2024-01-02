package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeRejectedFrame encodes RejectedFrame to Y3 encoded bytes.
func encodeRejectedFrame(f *frame.RejectedFrame) ([]byte, error) {
	// message
	messageBlock := y3.NewPrimitivePacketEncoder(tagRejectedMessage)
	messageBlock.SetStringValue(f.Message)
	// frame
	ff := y3.NewNodePacketEncoder(byte(f.Type()))
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
	tagRejectedMessage byte = 0x01
)
