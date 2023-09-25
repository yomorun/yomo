package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeDataFrame returns Y3 encoded bytes of DataFrame.
func encodeDataFrame(f *frame.DataFrame) ([]byte, error) {
	// tag
	tagBlock := y3.NewPrimitivePacketEncoder(tagDataFrameTag)
	tagBlock.SetUInt32Value(f.Tag)

	// metadata
	metadataBlock := y3.NewPrimitivePacketEncoder(tagDataFrameMetadata)
	metadataBlock.SetBytesValue(f.Metadata)

	// payload
	payloadBlock := y3.NewPrimitivePacketEncoder(tagDataFramePayload)
	payloadBlock.SetBytesValue(f.Payload)

	// data frame
	data := y3.NewNodePacketEncoder(byte(f.Type()))
	data.AddPrimitivePacket(tagBlock)
	data.AddPrimitivePacket(metadataBlock)
	data.AddPrimitivePacket(payloadBlock)

	return data.Encode(), nil
}

// decodeDataFrame decode Y3 encoded bytes to `DataFrame`
func decodeDataFrame(data []byte, f *frame.DataFrame) error {
	packet := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &packet)
	if err != nil {
		return err
	}

	// tag
	if tagBlock, ok := packet.PrimitivePackets[byte(tagDataFrameTag)]; ok {
		tag, err := tagBlock.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	// metadata
	if metadataBlock, ok := packet.PrimitivePackets[byte(tagDataFrameMetadata)]; ok {
		metadata := metadataBlock.ToBytes()
		f.Metadata = metadata
	}

	// payload
	if payloadBlock, ok := packet.PrimitivePackets[byte(tagDataFramePayload)]; ok {
		payload := payloadBlock.ToBytes()
		f.Payload = payload
	}

	return nil
}

var (
	tagDataFrameTag      byte = 0x01
	tagDataFramePayload  byte = 0x02
	tagDataFrameMetadata byte = 0x03
)
