package frame

import "github.com/yomorun/y3"

// AuthenticationRespFrame is the response of Authentication.
// AuthenticationRespFrame is a Y3 encoded bytes.
type AuthenticationRespFrame struct {
	ok     bool
	reason string
}

// OK returns if Authentication is success.
func (f *AuthenticationRespFrame) OK() bool { return f.ok }

// Reason returns the failed reason of Authentication.
func (f *AuthenticationRespFrame) Reason() string { return f.reason }

// NewAuthenticationRespFrame returns a AuthenticationRespFrame.
func NewAuthenticationRespFrame(ok bool, reason string) *AuthenticationRespFrame {
	return &AuthenticationRespFrame{
		ok:     ok,
		reason: reason,
	}
}

// Type gets the type of the AuthenticationRespFrame.
func (f *AuthenticationRespFrame) Type() Type {
	return TagOfAuthenticationRespFrame
}

// Encode encodes AuthenticationRespFrame to Y3 encoded bytes.
func (f *AuthenticationRespFrame) Encode() []byte {
	// ok
	okBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationRespOk))
	okBlock.SetBoolValue(f.ok)
	// reason
	reasonBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationRespReason))
	reasonBlock.SetStringValue(f.reason)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(okBlock)
	ack.AddPrimitivePacket(reasonBlock)

	return ack.Encode()
}

// DecodeToAuthenticationRespFrame decodes Y3 encoded bytes to AuthenticationRespFrame.
func DecodeToAuthenticationRespFrame(buf []byte) (*AuthenticationRespFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	f := &AuthenticationRespFrame{}

	// ok
	if okBlock, ok := node.PrimitivePackets[byte(TagOfAuthenticationRespOk)]; ok {
		ok, err := okBlock.ToBool()
		if err != nil {
			return nil, err
		}
		f.ok = ok
	}
	// reason
	if reasonBlock, ok := node.PrimitivePackets[byte(TagOfAuthenticationRespReason)]; ok {
		reason, err := reasonBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.reason = reason
	}

	return f, nil
}
