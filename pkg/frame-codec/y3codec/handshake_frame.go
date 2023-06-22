package y3codec

import (
	"encoding/binary"

	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeHandshakeFrame encodes HandshakeFrame to bytes in Y3 codec.
func encodeHandshakeFrame(f *frame.HandshakeFrame) ([]byte, error) {
	// name
	nameBlock := y3.NewPrimitivePacketEncoder(tagHandshakeName)
	nameBlock.SetStringValue(f.Name)
	// ID
	idBlock := y3.NewPrimitivePacketEncoder(tagHandshakeID)
	idBlock.SetStringValue(f.ID)
	// stream type
	typeBlock := y3.NewPrimitivePacketEncoder(tagHandshakeStreamType)
	typeBlock.SetBytesValue([]byte{f.StreamType})
	// observe data tags
	observeDataTagsBlock := y3.NewPrimitivePacketEncoder(tagHandshakeObserveDataTags)
	buf := make([]byte, 4)
	for _, v := range f.ObserveDataTags {
		binary.LittleEndian.PutUint32(buf, uint32(v))
		observeDataTagsBlock.AddBytes(buf)
	}
	// metadata
	metadataBlock := y3.NewPrimitivePacketEncoder(tagHandshakeMetadata)
	metadataBlock.SetBytesValue(f.Metadata)
	// handshake frame
	handshake := y3.NewNodePacketEncoder(byte(f.Type()))
	handshake.AddPrimitivePacket(nameBlock)
	handshake.AddPrimitivePacket(idBlock)
	handshake.AddPrimitivePacket(typeBlock)
	handshake.AddPrimitivePacket(observeDataTagsBlock)
	handshake.AddPrimitivePacket(metadataBlock)

	return handshake.Encode(), nil
}

// decodeHandshakeFrame decodes HandshakeFrame from bytes.
func decodeHandshakeFrame(data []byte, f *frame.HandshakeFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// name
	if nameBlock, ok := node.PrimitivePackets[byte(tagHandshakeName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Name = name
	}
	// ID
	if idBlock, ok := node.PrimitivePackets[byte(tagHandshakeID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.ID = id
	}
	// stream type
	if typeBlock, ok := node.PrimitivePackets[byte(tagHandshakeStreamType)]; ok {
		streamType := typeBlock.ToBytes()
		f.StreamType = streamType[0]
	}
	// observe data tag list
	if observeDataTagsBlock, ok := node.PrimitivePackets[byte(tagHandshakeObserveDataTags)]; ok {
		buf := observeDataTagsBlock.GetValBuf()
		length := len(buf) / 4
		for i := 0; i < length; i++ {
			pos := i * 4
			f.ObserveDataTags = append(f.ObserveDataTags, frame.Tag(binary.LittleEndian.Uint32(buf[pos:pos+4])))
		}
	}
	// metadata
	if typeBlock, ok := node.PrimitivePackets[byte(tagHandshakeMetadata)]; ok {
		metadata := typeBlock.ToBytes()
		f.Metadata = metadata
	}

	return nil
}

var (
	tagHandshakeName            byte = 0x01
	tagHandshakeStreamType      byte = 0x02
	tagHandshakeID              byte = 0x03
	tagHandshakeObserveDataTags byte = 0x06
	tagHandshakeMetadata        byte = 0x07
)
