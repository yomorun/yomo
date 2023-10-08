package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeRequestStreamFrame RequestStreamframe to Y3 encoded bytes
func encodeRequestStreamFrame(f *frame.RequestStreamFrame) ([]byte, error) {
	// client id
	clientID := y3.NewPrimitivePacketEncoder(tagRequestStreamFrameClientID)
	clientID.SetStringValue(f.ClientID)
	// tag
	tag := y3.NewPrimitivePacketEncoder(tagRequestStreamFrameTag)
	tag.SetUInt32Value(f.Tag)
	// encode
	node := y3.NewNodePacketEncoder(byte(f.Type()))
	node.AddPrimitivePacket(clientID)
	node.AddPrimitivePacket(tag)

	return node.Encode(), nil
}

// decodeRequestStreamFrame decodes Y3 encoded bytes to RequestStreamFrame.
func decodeRequestStreamFrame(data []byte, f *frame.RequestStreamFrame) error {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &nodeBlock)
	if err != nil {
		return err
	}
	// client id
	if p, ok := nodeBlock.PrimitivePackets[tagRequestStreamFrameClientID]; ok {
		clientID, err := p.ToUTF8String()
		if err != nil {
			return err
		}
		f.ClientID = clientID
	}
	// tag
	if p, ok := nodeBlock.PrimitivePackets[byte(tagRequestStreamFrameTag)]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return err
		}
		f.Tag = tag
	}

	return nil
}

var (
	tagRequestStreamFrameClientID byte = 0x02
	tagRequestStreamFrameTag      byte = 0x05
)
