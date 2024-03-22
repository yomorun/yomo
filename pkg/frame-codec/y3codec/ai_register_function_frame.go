package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeAIRegisterFunctionFrame encodes AIRegisterFunctionFrame to bytes in Y3 codec.
func encodeAIRegisterFunctionFrame(f *frame.AIRegisterFunctionFrame) ([]byte, error) {
	// name
	nameBlock := y3.NewPrimitivePacketEncoder(tagAIRegisterFunctionName)
	nameBlock.SetStringValue(f.Name)
	// tag
	tagBlock := y3.NewPrimitivePacketEncoder(tagAIRegisterFunctionTag)
	tagBlock.SetUInt32Value(f.Tag)
	// definition
	definitionBlock := y3.NewPrimitivePacketEncoder(tagAIRegisterFunctionDefinition)
	definitionBlock.SetBytesValue(f.Definition)
	// frame
	encoder := y3.NewNodePacketEncoder(byte(f.Type()))
	encoder.AddPrimitivePacket(nameBlock)
	encoder.AddPrimitivePacket(tagBlock)
	encoder.AddPrimitivePacket(definitionBlock)

	return encoder.Encode(), nil
}

// decodeAIRegisterFunctionFrame decodes bytes to AIRegisterFunctionFrame in Y3 codec.
func decodeAIRegisterFunctionFrame(data []byte, f *frame.AIRegisterFunctionFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	// name
	if nameBlock, ok := node.PrimitivePackets[byte(tagAIRegisterFunctionName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Name = name
	}
	// tag
	if tagBlock, ok := node.PrimitivePackets[byte(tagAIRegisterFunctionTag)]; ok {
		tag, err := tagBlock.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = tag
	}
	// definition
	if definitionBlock, ok := node.PrimitivePackets[byte(tagAIRegisterFunctionDefinition)]; ok {
		f.Definition = definitionBlock.ToBytes()
	}
	return nil
}

const (
	tagAIRegisterFunctionName       byte = 0x01
	tagAIRegisterFunctionTag        byte = 0x02
	tagAIRegisterFunctionDefinition byte = 0x03
)
