package frame

import "github.com/yomorun/y3"

// CloseStreamFrame is used to close a dataStream, controlStream
// receives CloseStreamFrame and closes dataStream according to the Frame.
// CloseStreamFrame is a Y3 encoded bytes.
type CloseStreamFrame struct {
	streamID string
	reason   string
}

// StreamID returns the ID of the stream to be closed.
func (f *CloseStreamFrame) StreamID() string { return f.streamID }

// Reason returns the close reason.
func (f *CloseStreamFrame) Reason() string { return f.reason }

// NewCloseStreamFrame returns a CloseStreamFrame.
func NewCloseStreamFrame(streamID, reason string) *CloseStreamFrame {
	return &CloseStreamFrame{
		streamID: streamID,
		reason:   reason,
	}
}

// Type gets the type of the CloseStreamFrame.
func (f *CloseStreamFrame) Type() Type {
	return TagOfCloseStreamFrame
}

// Encode encodes CloseStreamFrame to Y3 encoded bytes.
func (f *CloseStreamFrame) Encode() []byte {
	// id
	idBlock := y3.NewPrimitivePacketEncoder(byte(TagOfCloseStreamID))
	idBlock.SetStringValue(f.streamID)
	// reason
	reasonBlock := y3.NewPrimitivePacketEncoder(byte(TagOfCloseStreamReason))
	reasonBlock.SetStringValue(f.reason)
	// frame
	ack := y3.NewNodePacketEncoder(byte(f.Type()))
	ack.AddPrimitivePacket(idBlock)
	ack.AddPrimitivePacket(reasonBlock)

	return ack.Encode()
}

// DecodeToCloseStreamFrame decodes Y3 encoded bytes to CloseStreamFrame.
func DecodeToCloseStreamFrame(buf []byte) (*CloseStreamFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	f := &CloseStreamFrame{}

	// id
	if idBlock, ok := node.PrimitivePackets[byte(TagOfCloseStreamID)]; ok {
		id, err := idBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.streamID = id
	}
	// reason
	if reasonBlock, ok := node.PrimitivePackets[byte(TagOfCloseStreamReason)]; ok {
		reason, err := reasonBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		f.reason = reason
	}

	return f, nil
}
