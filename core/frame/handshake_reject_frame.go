package frame

import "github.com/yomorun/y3"

// HandshakeRejectFrame be used to reject a Handshake.
// HandshakeRejectFrame is a Y3 encoded bytes.
type HandshakeRejectFrame struct {
	streamID string
	reason   string
}

// StreamID returns the streamID of handshake be rejected.
func (f *HandshakeRejectFrame) StreamID() string { return f.streamID }

// Reason returns the reject reason of Handshake.
func (f *HandshakeRejectFrame) Reason() string { return f.reason }

// NewHandshakeRejectFrame returns a HandshakeRejectFrame.
func NewHandshakeRejectFrame(streamID, reason string) *HandshakeRejectFrame {
	return &HandshakeRejectFrame{
		streamID: streamID,
		reason:   reason,
	}
}

// Type gets the type of the HandshakeRejectFrame.
func (f *HandshakeRejectFrame) Type() Type {
	return TagOfHandshakeRejectFrame
}

// Encode encodes HandshakeRejectFrame to Y3 encoded bytes.
func (f *HandshakeRejectFrame) Encode() []byte {
	// id
	idBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeRejectStreamID))
	idBlock.SetStringValue(f.streamID)
	// reason
	reasonBlock := y3.NewPrimitivePacketEncoder(byte(TagOfAuthenticationRespReason))
	reasonBlock.SetStringValue(f.reason)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(idBlock)
	ack.AddPrimitivePacket(reasonBlock)

	return ack.Encode()
}

// DecodeToHandshakeRejectFrame decodes Y3 encoded bytes to HandshakeRejectFrame.
func DecodeToHandshakeRejectFrame(buf []byte) (*HandshakeRejectFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	f := &HandshakeRejectFrame{}

	// id
	if idBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeRejectStreamID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.streamID = id
	}
	// reason
	if reasonBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeRejectReason)]; ok {
		reason, err := reasonBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.reason = reason
	}

	return f, nil
}
