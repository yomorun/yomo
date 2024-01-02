package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeHandshakeAckFrame encodes HandshakeAckFrame to Y3 encoded bytes.
func encodeHandshakeAckFrame(f *frame.HandshakeAckFrame) ([]byte, error) {
	// message
	messageBlock := y3.NewPrimitivePacketEncoder(tagHandshakeAckMessage)
	messageBlock.SetStringValue(f.Message)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(messageBlock)

	return ack.Encode(), nil
}

// decodeHandshakeAckFrame decodes Y3 encoded bytes to HandshakeAckFrame
func decodeHandshakeAckFrame(data []byte, f *frame.HandshakeAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	// message
	if messageBlock, ok := node.PrimitivePackets[tagHandshakeAckMessage]; ok {
		message, err := messageBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Message = message
	}
	return nil
}

var (
	tagHandshakeAckMessage byte = 0x01
)
