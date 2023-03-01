package frame

import (
	"github.com/yomorun/y3"
)

// AuthenticationFrame is used to authenticate the client,
// Once the connection is established, the client immediately, sends information
// to the server, server gets the way to authenticate according to authName and
// use authPayload to do a authentication.
//
//	AuthenticationFrame is a Y3 encoded.
type AuthenticationFrame struct {
	authName    string
	authPayload string
}

// NewAuthenticationFrame creates a new AuthenticationFrame.
func NewAuthenticationFrame(authName string, authPayload string) *AuthenticationFrame {
	return &AuthenticationFrame{
		authName:    authName,
		authPayload: authPayload,
	}
}

// Type returns the type of AuthenticationFrame.
func (h *AuthenticationFrame) Type() Type {
	return TagOfAuthenticationFrame
}

// Encode encodes AuthenticationFrame to bytes in Y3 codec.
func (h *AuthenticationFrame) Encode() []byte {
	// auth
	authNameBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationName))
	authNameBlock.SetStringValue(h.authName)
	authPayloadBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationPayload))
	authPayloadBlock.SetStringValue(h.authPayload)
	// authentication frame
	authentication := y3.NewNodePacketEncoder(byte(h.Type()))
	authentication.AddPrimitivePacket(authNameBlock)
	authentication.AddPrimitivePacket(authPayloadBlock)

	return authentication.Encode()
}

// DecodeToAuthenticationFrame decodes Y3 encoded bytes to AuthenticationFrame.
func DecodeToAuthenticationFrame(buf []byte) (*AuthenticationFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	authentication := &AuthenticationFrame{}

	// auth
	if authNameBlock, ok := node.PrimitivePackets[byte(TagOfAuthenticationName)]; ok {
		authName, err := authNameBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		authentication.authName = authName
	}
	if authPayloadBlock, ok := node.PrimitivePackets[byte(TagOfAuthenticationPayload)]; ok {
		authPayload, err := authPayloadBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		authentication.authPayload = authPayload
	}

	return authentication, nil
}

// AuthPayload returns authentication payload.
func (h *AuthenticationFrame) AuthPayload() string {
	return h.authPayload
}

// AuthName returns authentication name,
// server finds the mode of authentication in AuthName.
func (h *AuthenticationFrame) AuthName() string {
	return h.authName
}
