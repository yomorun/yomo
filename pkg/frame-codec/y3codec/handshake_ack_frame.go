package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeHandshakeAckFrame encodes HandshakeAckFrame to Y3 encoded bytes.
func encodeHandshakeAckFrame(f *frame.HandshakeAckFrame) ([]byte, error) {
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	// streamID
	streamIDBlock := y3.NewPrimitivePacketEncoder(tagHandshakeAckStreamID)
	streamIDBlock.SetStringValue(f.StreamID)

	ack.AddPrimitivePacket(streamIDBlock)

	return ack.Encode(), nil
}

// decodeHandshakeAckFrame decodes Y3 encoded bytes to HandshakeAckFrame
func decodeHandshakeAckFrame(data []byte, f *frame.HandshakeAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// streamID
	if streamIDBlock, ok := node.PrimitivePackets[tagHandshakeAckStreamID]; ok {
		streamID, err := streamIDBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.StreamID = streamID
	}
	return nil
}

var tagHandshakeAckStreamID byte = 0x28
