package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeHandshakeRejectedFrame encodes HandshakeRejectedFrame to Y3 encoded bytes.
func encodeHandshakeRejectedFrame(f *frame.HandshakeRejectedFrame) ([]byte, error) {
	// id
	idBlock := y3.NewPrimitivePacketEncoder(byte(tagHandshakeRejectedStreamID))
	idBlock.SetStringValue(f.ID)
	// message
	messageBlock := y3.NewPrimitivePacketEncoder(tagHandshakeRejectedMessage)
	messageBlock.SetStringValue(f.Message)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(idBlock)
	ack.AddPrimitivePacket(messageBlock)

	return ack.Encode(), nil
}

// decodeHandshakeRejectedFrame decodes Y3 encoded bytes to HandshakeRejectedFrame.
func decodeHandshakeRejectedFrame(data []byte, f *frame.HandshakeRejectedFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// id
	if idBlock, ok := node.PrimitivePackets[tagHandshakeRejectedStreamID]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.ID = id
	}
	// message
	if messageBlock, ok := node.PrimitivePackets[tagHandshakeRejectedMessage]; ok {
		message, err := messageBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Message = message
	}

	return nil
}

var (
	tagHandshakeRejectedStreamID byte = 0x15
	tagHandshakeRejectedMessage  byte = 0x16
)
