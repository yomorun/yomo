package y3codec

import (
	"github.com/yomorun/y3"
	frame "github.com/yomorun/yomo/core/frame"
)

// encodeAuthenticationFrame encodes AuthenticationFrame to bytes in Y3 codec.
func encodeAuthenticationFrame(f *frame.AuthenticationFrame) ([]byte, error) {
	// auth
	authNameBlock := y3.NewPrimitivePacketEncoder(tagAuthenticationName)
	authNameBlock.SetStringValue(f.AuthName)
	authPayloadBlock := y3.NewPrimitivePacketEncoder(tagAuthenticationPayload)
	authPayloadBlock.SetStringValue(f.AuthPayload)
	// authentication frame
	authentication := y3.NewNodePacketEncoder(byte(f.Type()))
	authentication.AddPrimitivePacket(authNameBlock)
	authentication.AddPrimitivePacket(authPayloadBlock)

	return authentication.Encode(), nil
}

// decodeAuthenticationFrame decodes Y3 encoded bytes to AuthenticationFrame.
func decodeAuthenticationFrame(data []byte, f *frame.AuthenticationFrame) error {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(data, &node)
	if err != nil {
		return err
	}
	// auth
	if authNameBlock, ok := node.PrimitivePackets[tagAuthenticationName]; ok {
		authName, err := authNameBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.AuthName = authName
	}
	// payload
	if authPayloadBlock, ok := node.PrimitivePackets[tagAuthenticationPayload]; ok {
		authPayload, err := authPayloadBlock.ToUTF8String()
		if err != nil {
			return err
		}
		f.AuthPayload = authPayload
	}

	return nil
}

var (
	tagAuthenticationName    byte = 0x04
	tagAuthenticationPayload byte = 0x05
)
