package frame

import (
	"encoding/binary"

	"github.com/yomorun/y3"
)

type ConnectionFrame struct {
	// Name is the name of connection.
	Name string
	// ClientID represents client ID.
	ClientID string
	// ClientType represents client type (source, sfn or upStream).
	ClientType byte
	// ObserveDataTags are the client data tag list.
	ObserveDataTags []Tag
	// Metadata holds Connection metadata.
	Metadata []byte
}

// Type returns the type of ConnectionFrame.
func (f *ConnectionFrame) Type() Type {
	return TagOfConnectionFrame
}

func (h *ConnectionFrame) Encode() []byte {
	// name
	nameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfConnectionName))
	nameBlock.SetStringValue(h.Name)
	// client ID
	idBlock := y3.NewPrimitivePacketEncoder(byte(TagOfConnectionID))
	idBlock.SetStringValue(h.ClientID)
	// client type
	typeBlock := y3.NewPrimitivePacketEncoder(byte(TagOfConnectionType))
	typeBlock.SetBytesValue([]byte{h.ClientType})
	// observe data tags
	observeDataTagsBlock := y3.NewPrimitivePacketEncoder(byte(TagOfConnectionObserveDataTags))
	buf := make([]byte, 4)
	for _, v := range h.ObserveDataTags {
		binary.LittleEndian.PutUint32(buf, uint32(v))
		observeDataTagsBlock.AddBytes(buf)
	}
	// connection frame
	connection := y3.NewNodePacketEncoder(byte(h.Type()))
	connection.AddPrimitivePacket(nameBlock)
	connection.AddPrimitivePacket(idBlock)
	connection.AddPrimitivePacket(typeBlock)
	connection.AddPrimitivePacket(observeDataTagsBlock)

	return connection.Encode()
}

func DecodeToConnectionFrame(buf []byte) (*ConnectionFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	connection := &ConnectionFrame{}
	// name
	if nameBlock, ok := node.PrimitivePackets[byte(TagOfConnectionName)]; ok {
		name, err := nameBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		connection.Name = name
	}
	// client ID
	if idBlock, ok := node.PrimitivePackets[byte(TagOfConnectionID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		connection.ClientID = id
	}
	// client type
	if typeBlock, ok := node.PrimitivePackets[byte(TagOfConnectionType)]; ok {
		clientType := typeBlock.ToBytes()
		connection.ClientType = clientType[0]
	}
	// observe data tag list
	if observeDataTagsBlock, ok := node.PrimitivePackets[byte(TagOfConnectionObserveDataTags)]; ok {
		buf := observeDataTagsBlock.GetValBuf()
		length := len(buf) / 4
		for i := 0; i < length; i++ {
			pos := i * 4
			connection.ObserveDataTags = append(connection.ObserveDataTags, Tag(binary.LittleEndian.Uint32(buf[pos:pos+4])))
		}
	}

	return connection, nil
}
