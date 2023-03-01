package frame

import (
	"encoding/binary"

	"github.com/yomorun/y3"
)

// HandshakeFrame is the frame that client accquires new dataStream from server,
// It includes some of the information necessary to create a new dataStream.
// Based on this information, the server can create dataStream。
type HandshakeFrame struct {
	// Name is the name of dataStream.
	Name string

	// ID represents the dataStream ID, the ID must be an unique string.
	ID string

	// StreamType represents dataStream type (Source | SFN | UpstreamZipper).
	// different StreamType has different behaviors in server side.
	StreamType byte

	// ObserveDataTags are the stream data tag list.
	ObserveDataTags []Tag

	// Metadata holds stream metadata,
	// metadata stores information for route the data.
	Metadata []byte
}

// Type returns the type of HandshakeFrame.
func (f *HandshakeFrame) Type() Type {
	return TagOfHandshakeFrame
}

// Encode encodes HandshakeFrame to bytes in Y3 codec.
func (h *HandshakeFrame) Encode() []byte {
	// name
	nameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeName))
	nameBlock.SetStringValue(h.Name)
	// ID
	idBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeID))
	idBlock.SetStringValue(h.ID)
	// stream type
	typeBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeStreamType))
	typeBlock.SetBytesValue([]byte{h.StreamType})
	// observe data tags
	observeDataTagsBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeObserveDataTags))
	buf := make([]byte, 4)
	for _, v := range h.ObserveDataTags {
		binary.LittleEndian.PutUint32(buf, uint32(v))
		observeDataTagsBlock.AddBytes(buf)
	}
	// metadata
	metadataBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeMetadata))
	metadataBlock.SetBytesValue(h.Metadata)
	// handshake frame
	handshake := y3.NewNodePacketEncoder(byte(h.Type()))
	handshake.AddPrimitivePacket(nameBlock)
	handshake.AddPrimitivePacket(idBlock)
	handshake.AddPrimitivePacket(typeBlock)
	handshake.AddPrimitivePacket(observeDataTagsBlock)
	handshake.AddPrimitivePacket(metadataBlock)

	return handshake.Encode()
}

// DecodeToHandshakeFrame decodes HandshakeFrame from bytes.
func DecodeToHandshakeFrame(buf []byte) (*HandshakeFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	handshake := &HandshakeFrame{}
	// name
	if nameBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.Name = name
	}
	// ID
	if idBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		handshake.ID = id
	}
	// stream type
	if typeBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeStreamType)]; ok {
		streamType := typeBlock.ToBytes()
		handshake.StreamType = streamType[0]
	}
	// observe data tag list
	if observeDataTagsBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeObserveDataTags)]; ok {
		buf := observeDataTagsBlock.GetValBuf()
		length := len(buf) / 4
		for i := 0; i < length; i++ {
			pos := i * 4
			handshake.ObserveDataTags = append(handshake.ObserveDataTags, Tag(binary.LittleEndian.Uint32(buf[pos:pos+4])))
		}
	}
	// metadata
	if typeBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeMetadata)]; ok {
		metadata := typeBlock.ToBytes()
		handshake.Metadata = metadata
	}

	return handshake, nil
}
