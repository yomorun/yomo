package frame

import "github.com/yomorun/y3"

// AuthenticationAckFrame is used to ack Authentication.
// HandshakeAckFrame is a Y3 encoded bytes.
type AuthenticationAckFrame struct {
	ok     bool
	reason string
}

// OK returns if Authentication is success.
func (f *AuthenticationAckFrame) OK() bool { return f.ok }

// Reason returns the reason for Authentication.
func (f *AuthenticationAckFrame) Reason() string { return f.reason }

// NewAuthenticationAckFrame returns a AuthenticationAckFrame.
func NewAuthenticationAckFrame(ok bool, reason string) *AuthenticationAckFrame {
	return &AuthenticationAckFrame{
		ok:     ok,
		reason: reason,
	}
}

// Type gets the type of the AuthenticationAckFrame.
func (f *AuthenticationAckFrame) Type() Type {
	return TagOfAuthenticationAckFrame
}

// Encode encodes AuthenticationAckFrame to Y3 encoded bytes.
func (f *AuthenticationAckFrame) Encode() []byte {
	// ok
	okBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationAckOk))
	okBlock.SetBoolValue(f.ok)
	// reason
	reasonBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationAckReason))
	reasonBlock.SetStringValue(f.reason)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(okBlock)
	ack.AddPrimitivePacket(reasonBlock)

	return ack.Encode()
}

// DecodeToAuthenticationAckFrame decodes Y3 encoded bytes to AuthenticationAckFrame.
func DecodeToAuthenticationAckFrame(buf []byte) (*AuthenticationAckFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	f := &AuthenticationAckFrame{}

	// ok
	if okBlock, ok := node.PrimitivePackets[byte(TagOfAuthenticationAckOk)]; ok {
		ok, err := okBlock.ToBool()
		if err != nil {
			return nil, err
		}
		f.ok = ok
	}
	// reason
	if reasonBlock, ok := node.PrimitivePackets[byte(TagOfAuthenticationAckReason)]; ok {
		reason, err := reasonBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.reason = reason
	}

	return f, nil
}
