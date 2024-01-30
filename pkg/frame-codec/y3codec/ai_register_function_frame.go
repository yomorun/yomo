package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeAIRegisterFunctionAckFrame encodes AIRegisterFunctionAckFrame to bytes in Y3 codec.
func encodeAIRegisterFunctionAckFrame(f *frame.AIRegisterFunctionAckFrame) ([]byte, error) {
	encoder := y3.NewNodePacketEncoder(byte(f.Type()))
	return encoder.Encode(), nil
}

// decodeAIRegisterFunctionAckFrame decodes bytes to AIRegisterFunctionAckFrame in Y3 codec.
func decodeAIRegisterFunctionAckFrame(data []byte, f *frame.AIRegisterFunctionAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	return nil
}
