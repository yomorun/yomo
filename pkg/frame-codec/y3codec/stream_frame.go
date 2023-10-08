package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeStreamFrame Streamframe to Y3 encoded bytes
func encodeStreamFrame(f *frame.StreamFrame) ([]byte, error) {
	// id
	id := y3.NewPrimitivePacketEncoder(tagStreamFrameID)
	id.SetStringValue(f.ID)
	// client id
	clientID := y3.NewPrimitivePacketEncoder(tagStreamFrameClientID)
	clientID.SetStringValue(f.ClientID)
	// stream id
	streamID := y3.NewPrimitivePacketEncoder(tagStreamFrameStreamID)
	streamID.SetInt64Value(f.StreamID)
	// chunk size
	chunkSize := y3.NewPrimitivePacketEncoder(tagStreamFrameChunkSize)
	chunkSize.SetUInt32Value(uint32(f.ChunkSize))
	// tag
	tag := y3.NewPrimitivePacketEncoder(tagStreamFrameTag)
	tag.SetUInt32Value(f.Tag)
	// encode
	node := y3.NewNodePacketEncoder(byte(f.Type()))
	node.AddPrimitivePacket(id)
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
	// id
	if p, ok := nodeBlock.PrimitivePackets[tagStreamFrameID]; ok {
		id, err := p.ToUTF8String()
		if err != nil {
			return err
		}
		f.ID = id
	}
	// client id
	if p, ok := nodeBlock.PrimitivePackets[tagStreamFrameClientID]; ok {
		clientID, err := p.ToUTF8String()
		if err != nil {
			return err
		}
		f.ClientID = clientID
	}
	// stream id
	if p, ok := nodeBlock.PrimitivePackets[tagStreamFrameStreamID]; ok {
		steamID, err := p.ToInt64()
		if err != nil {
			return err
		}
		f.StreamID = steamID
	}
	// chunk size
	if p, ok := nodeBlock.PrimitivePackets[tagStreamFrameChunkSize]; ok {
		chunkSize, err := p.ToInt32()
		if err != nil {
			return err
		}
		f.ChunkSize = uint(chunkSize)
	}
	// tag
	if p, ok := nodeBlock.PrimitivePackets[byte(tagStreamFrameTag)]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	return nil
}

var (
	tagStreamFrameID        byte = 0x01
	tagStreamFrameClientID  byte = 0x02
	tagStreamFrameStreamID  byte = 0x03
	tagStreamFrameChunkSize byte = 0x04
	tagStreamFrameTag       byte = 0x05
)
