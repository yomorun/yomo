package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeStreamFrame Streamframe to Y3 encoded bytes
func encodeStreamFrame(f *frame.StreamFrame) ([]byte, error) {
	clientID := y3.NewPrimitivePacketEncoder(tagStreamClientID)
	clientID.SetStringValue(f.ClientID)

	streamID := y3.NewPrimitivePacketEncoder(tagStreamID)
	streamID.SetInt64Value(f.StreamID)

	chunkSize := y3.NewPrimitivePacketEncoder(tagStreamChunkSize)
	chunkSize.SetUInt32Value(uint32(f.ChunkSize))

	tag := y3.NewPrimitivePacketEncoder(tagStreamTag)
	tag.SetUInt32Value(f.Tag)

	node := y3.NewNodePacketEncoder(byte(f.Type()))
	node.AddPrimitivePacket(clientID)
	node.AddPrimitivePacket(streamID)
	node.AddPrimitivePacket(chunkSize)
	node.AddPrimitivePacket(tag)

	return node.Encode(), nil
}

// decodeStreamFrame decodes Y3 encoded bytes to StreamFrame.
func decodeStreamFrame(data []byte, f *frame.StreamFrame) error {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &nodeBlock)
	if err != nil {
		return err
	}
	// client id
	if p, ok := nodeBlock.PrimitivePackets[tagStreamClientID]; ok {
		clientID, err := p.ToUTF8String()
		if err != nil {
			return err
		}
		f.ClientID = clientID
	}
	// stream id
	if p, ok := nodeBlock.PrimitivePackets[tagStreamID]; ok {
		steamID, err := p.ToInt64()
		if err != nil {
			return err
		}
		f.StreamID = steamID
	}
	// chunk size
	if p, ok := nodeBlock.PrimitivePackets[tagStreamChunkSize]; ok {
		chunkSize, err := p.ToInt32()
		if err != nil {
			return err
		}
		f.ChunkSize = uint(chunkSize)
	}

	// tag
	if p, ok := nodeBlock.PrimitivePackets[byte(tagStreamTag)]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	return nil
}

var (
	tagStreamClientID  byte = 0x01
	tagStreamID        byte = 0x02
	tagStreamChunkSize byte = 0x03
	tagStreamTag       byte = 0x04
)
