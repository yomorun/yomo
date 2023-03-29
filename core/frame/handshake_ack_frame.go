package frame

import "github.com/yomorun/y3"

// HandshakeAckFrame is used to ack handshake, It is always that the first frame
// is HandshakeAckFrame after client acquire a new stream.
// HandshakeAckFrame is a Y3 encoded bytes.
type HandshakeAckFrame struct {
	streamID string
}

// NewHandshakeAckFrame returns a HandshakeAckFrame.
func NewHandshakeAckFrame(streamID string) *HandshakeAckFrame {
	return &HandshakeAckFrame{streamID}
}

// Type gets the type of the HandshakeAckFrame.
func (f *HandshakeAckFrame) Type() Type {
	return TagOfHandshakeAckFrame
}

// StreamID returns the id of stream be acked.
func (f *HandshakeAckFrame) StreamID() string {
	return f.streamID
}

// Encode encodes HandshakeAckFrame to Y3 encoded bytes.
func (f *HandshakeAckFrame) Encode() []byte {
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	// streamID
	streamIDBlock := y3.NewPrimitivePacketEncoder(byte(TagOfHandshakeAckStreamID))
	streamIDBlock.SetStringValue(f.streamID)

	ack.AddPrimitivePacket(streamIDBlock)

	return ack.Encode()
}

// DecodeToHandshakeAckFrame decodes Y3 encoded bytes to HandshakeAckFrame
func DecodeToHandshakeAckFrame(buf []byte) (*HandshakeAckFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	ack := &HandshakeAckFrame{}
	// streamID
	if streamIDBlock, ok := node.PrimitivePackets[byte(TagOfHandshakeAckStreamID)]; ok {
		streamID, err := streamIDBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		ack.streamID = streamID
	}
	return ack, nil
}
