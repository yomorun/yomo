package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeHandshakeAckFrame encodes HandshakeAckFrame to Y3 encoded bytes.
func encodeHandshakeAckFrame(f *frame.HandshakeAckFrame) ([]byte, error) {
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	return ack.Encode(), nil
}

// decodeHandshakeAckFrame decodes Y3 encoded bytes to HandshakeAckFrame
func decodeHandshakeAckFrame(data []byte, _ *frame.HandshakeAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	return nil
}
