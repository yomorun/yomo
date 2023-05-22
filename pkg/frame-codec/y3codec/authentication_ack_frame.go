package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeAuthenticationAckFrame encodes AuthenticationAckFrame to Y3 encoded bytes.
func encodeAuthenticationAckFrame(f *frame.AuthenticationAckFrame) ([]byte, error) {
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))

	return ack.Encode(), nil
}

// decodeAuthenticationAckFrame decodes Y3 encoded bytes to AuthenticationAckFrame.
func decodeAuthenticationAckFrame(data []byte, f *frame.AuthenticationAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	return nil
}
