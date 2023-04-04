package frame

import "github.com/yomorun/y3"

// HandshakeRejectedFrame be used to reject a Handshake.
// HandshakeRejectedFrame is a Y3 encoded bytes.
type HandshakeRejectedFrame struct {
	streamID string
	reason   string
}

// StreamID returns the streamID of handshake be rejected.
func (f *HandshakeRejectedFrame) StreamID() string { return f.streamID }

// Reason returns the reason for the rejection of a Handshake, if it was rejected.
func (f *HandshakeRejectedFrame) Reason() string { return f.reason }

// NewHandshakeRejectedFrame returns a HandshakeRejectedFrame.
func NewHandshakeRejectedFrame(streamID, reason string) *HandshakeRejectedFrame {
	return &HandshakeRejectedFrame{
		streamID: streamID,
		reason:   reason,
	}
}

// Type gets the type of the HandshakeRejectedFrame.
func (f *HandshakeRejectedFrame) Type() Type {
	return TagOfHandshakeRejectedFrame
}

// Encode encodes HandshakeRejectedFrame to Y3 encoded bytes.
func (f *HandshakeRejectedFrame) Encode() []byte {
	// id
	idBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeRejectedStreamID))
	idBlock.SetStringValue(f.streamID)
	// reason
	reasonBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeRejectedReason))
	reasonBlock.SetStringValue(f.reason)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(idBlock)
	ack.AddPrimitivePacket(reasonBlock)

	return ack.Encode()
}

// DecodeToHandshakeRejectedFrame decodes Y3 encoded bytes to HandshakeRejectedFrame.
func DecodeToHandshakeRejectedFrame(buf []byte) (*HandshakeRejectedFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	f := &HandshakeRejectedFrame{}

	// id
	if idBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeRejectedStreamID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.streamID = id
	}
	// reason
	if reasonBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeRejectedReason)]; ok {
		reason, err := reasonBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.reason = reason
	}

	return f, nil
}
