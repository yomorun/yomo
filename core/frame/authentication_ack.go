package frame

import "github.com/yomorun/y3"

// AuthenticationAckFrame is the ack of Authentication.
// AuthenticationAckFrame is a Y3 encoded bytes.
type AuthenticationAckFrame struct{}

// NewAuthenticationAckFrame returns a AuthenticationAckFrame.
func NewAuthenticationAckFrame() *AuthenticationAckFrame {
	return &AuthenticationAckFrame{}
}

// Type gets the type of the AuthenticationAckFrame.
func (f *AuthenticationAckFrame) Type() Type {
	return TagOfAuthenticationAckFrame
}

// Encode encodes AuthenticationAckFrame to Y3 encoded bytes.
func (f *AuthenticationAckFrame) Encode() []byte {
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))

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

	return f, nil
}
