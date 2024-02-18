package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeHandshakeAckFrame encodes HandshakeAckFrame to Y3 encoded bytes.
func encodeHandshakeAckFrame(f *frame.HandshakeAckFrame) ([]byte, error) {
	appIDBlock := y3.NewPrimitivePacketEncoder(tagHandshakeAckAppID)
	appIDBlock.SetStringValue(f.AppID)
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(appIDBlock)
	return ack.Encode(), nil
}

// decodeHandshakeAckFrame decodes Y3 encoded bytes to HandshakeAckFrame
func decodeHandshakeAckFrame(data []byte, f *frame.HandshakeAckFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	// app id
	if appIDBlock, ok := node.PrimitivePackets[tagHandshakeAckAppID]; ok {
		appID, err := appIDBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.AppID = appID
	}
	return nil
}

const (
	tagHandshakeAckAppID byte = 0x01
)
