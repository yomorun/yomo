package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeMetaFrame returns Y3 encoded bytes of MetaFrame.
func encodeMetaFrame(f *frame.MetaFrame) ([]byte, error) {
	meta := y3.NewNodePacketEncoder(tagMetaFrame)
	// TID
	tid := y3.NewPrimitivePacketEncoder(byte(tagTID))
	tid.SetStringValue(f.TID)
	meta.AddPrimitivePacket(tid)
	// SID
	sid := y3.NewPrimitivePacketEncoder(byte(tagSID))
	sid.SetStringValue(f.SID)
	meta.AddPrimitivePacket(sid)
	// source ID
	sourceID := y3.NewPrimitivePacketEncoder(byte(tagSourceID))
	sourceID.SetStringValue(f.SourceID)
	meta.AddPrimitivePacket(sourceID)

	// metadata
	if len(f.Metadata) != 0 {
		metadata := y3.NewPrimitivePacketEncoder(byte(tagMetadata))
		metadata.SetBytesValue(f.Metadata)
		meta.AddPrimitivePacket(metadata)
	}

	// broadcast mode
	broadcast := y3.NewPrimitivePacketEncoder(byte(tagBroadcast))
	broadcast.SetBoolValue(f.Broadcast)
	meta.AddPrimitivePacket(broadcast)

	return meta.Encode(), nil
}

// decodeMetaFrame decodes a MetaFrame instance from given buffer.
func decodeMetaFrame(data []byte, f *frame.MetaFrame) error {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &nodeBlock)
	if err != nil {
		return err
	}

	for k, v := range nodeBlock.PrimitivePackets {
		switch k {
		case byte(tagTID):
			val, err := v.ToUTF8String()
			if err != nil {
				return err
			}
			f.TID = val
		case byte(tagSID):
			val, err := v.ToUTF8String()
			if err != nil {
				return err
			}
			f.SID = val
		case byte(tagMetadata):
			f.Metadata = v.ToBytes()
		case byte(tagSourceID):
			sourceID, err := v.ToUTF8String()
			if err != nil {
				return err
			}
			f.SourceID = sourceID
		case byte(tagBroadcast):
			broadcast, err := v.ToBool()
			if err != nil {
				return err
			}
			f.Broadcast = broadcast
		}
	}

	return nil
}

var (
	tagMetaFrame byte = 0x2F
	tagMetadata  byte = 0x03
	tagTID       byte = 0x01
	tagSourceID  byte = 0x02
	tagBroadcast byte = 0x04
	tagSID       byte = 0x05
)
