package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeAIRegisterFunctionAckFrame encodes AIRegisterFunctionAckFrame to bytes in Y3 codec.
func encodeAIRegisterFunctionAckFrame(f *frame.AIRegisterFunctionAckFrame) ([]byte, error) {
	// name
	nameBlock := y3.NewPrimitivePacketEncoder(tagAIRegisterFunctionAckName)
	nameBlock.SetStringValue(f.Name)
	// tag
	tagBlock := y3.NewPrimitivePacketEncoder(tagAIRegisterFunctionAckTag)
	tagBlock.SetUInt32Value(f.Tag)
	// encoder
	encoder := y3.NewNodePacketEncoder(byte(f.Type()))
	encoder.AddPrimitivePacket(nameBlock)
	encoder.AddPrimitivePacket(tagBlock)
	return encoder.Encode(), nil
}

// decodeAIRegisterFunctionAckFrame decodes bytes to AIRegisterFunctionAckFrame in Y3 codec.
func decodeAIRegisterFunctionAckFrame(data []byte, f *frame.AIRegisterFunctionAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	// name
	if nameBlock, ok := node.PrimitivePackets[byte(tagAIRegisterFunctionAckName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Name = name
	}
	// tag
	if tagBlock, ok := node.PrimitivePackets[byte(tagAIRegisterFunctionAckTag)]; ok {
		tag, err := tagBlock.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	return nil
}

const (
	tagAIRegisterFunctionAckName byte = 0x01
	tagAIRegisterFunctionAckTag  byte = 0x02
)
