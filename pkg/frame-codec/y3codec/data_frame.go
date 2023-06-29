package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeDataFrame returns Y3 encoded bytes of DataFrame.
func encodeDataFrame(f *frame.DataFrame) ([]byte, error) {
	data := y3.NewNodePacketEncoder(byte(f.Type()))
	// MetaFrame
	mb, _ := encodeMetaFrame(f.Meta)
	data.AddBytes(mb)

	// PayloadFrame
	pd, _ := encodePayloadFrame(f.Payload)
	data.AddBytes(pd)

	return data.Encode(), nil
}

// decodeDataFrame decode Y3 encoded bytes to `DataFrame`
func decodeDataFrame(data []byte, f *frame.DataFrame) error {
	packet := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &packet)
	if err != nil {
		return err
	}

	if metaBlock, ok := packet.NodePackets[tagMetaFrame]; ok {
		if f.Meta == nil {
			f.Meta = new(frame.MetaFrame)
		}
		err := decodeMetaFrame(metaBlock.GetRawBytes(), f.Meta)
		if err != nil {
			return err
		}
	}

	if payloadBlock, ok := packet.NodePackets[byte(frame.TypePayloadFrame)]; ok {
		if f.Payload == nil {
			f.Payload = new(frame.PayloadFrame)
		}
		err := decodeToPayloadFrame(payloadBlock.GetRawBytes(), f.Payload)
		if err != nil {
			return err
		}
	}

	return nil
}

var (
	tagPayloadDataTag  byte = 0x01
	tagPayloadCarriage byte = 0x02
)
