package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodePayloadFrame returns Y3 encoded bytes of PayloadFrame.
func encodePayloadFrame(f *frame.PayloadFrame) ([]byte, error) {
	tag := y3.NewPrimitivePacketEncoder(tagPayloadDataTag)
	tag.SetUInt32Value(f.Tag)

	carriage := y3.NewPrimitivePacketEncoder(tagPayloadCarriage)
	carriage.SetBytesValue(f.Carriage)

	payload := y3.NewNodePacketEncoder(byte(frame.TypePayloadFrame))
	payload.AddPrimitivePacket(tag)
	payload.AddPrimitivePacket(carriage)

	return payload.Encode(), nil
}

// decodePayloadFrame decodes Y3 encoded bytes to PayloadFrame.
func decodeToPayloadFrame(data []byte, payload *frame.PayloadFrame) error {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &nodeBlock)
	if err != nil {
		return err
	}

	if p, ok := nodeBlock.PrimitivePackets[byte(tagPayloadDataTag)]; ok {
		tag, err := p.ToUInt32()
		if err != nil {
			return err
		}
		payload.Tag = tag
	}

	if p, ok := nodeBlock.PrimitivePackets[byte(tagPayloadCarriage)]; ok {
		payload.Carriage = p.GetValBuf()
	}

	return nil
}
