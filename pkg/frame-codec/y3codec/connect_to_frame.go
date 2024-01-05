package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeConnectToFrame encodes ConnectToFrame to Y3 encoded bytes.
func encodeConnectToFrame(f *frame.ConnectToFrame) ([]byte, error) {
	// endpoint
	endpointBlock := y3.NewPrimitivePacketEncoder(tagConnectToEndpoint)
	endpointBlock.SetStringValue(f.Endpoint)
	// frame
	ff := y3.NewNodePacketEncoder(byte(f.Type()))
	ff.AddPrimitivePacket(endpointBlock)

	return ff.Encode(), nil
}

// decodeConnectToFrame decodes Y3 encoded bytes to ConnectToFrame.
func decodeConnectToFrame(data []byte, f *frame.ConnectToFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}

	// endpoint
	if endpointBlock, ok := node.PrimitivePackets[tagConnectToEndpoint]; ok {
		endpoint, err := endpointBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.Endpoint = endpoint
	}

	return nil
}

var (
	tagConnectToEndpoint byte = 0x01
)
