package frame

import "github.com/yomorun/y3"

// RejectedFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_REJECTED_FRAME
type RejectedFrame struct {
	message string
}

// NewRejectedFrame creates a new RejectedFrame with a given TagID of user's data
func NewRejectedFrame(msg string) *RejectedFrame {
	return &RejectedFrame{message: msg}
}

// Type gets the type of Frame.
func (f *RejectedFrame) Type() Type {
	return TagOfRejectedFrame
}

// Encode to Y3 encoded bytes
func (f *RejectedFrame) Encode() []byte {
	rejected := y3.NewNodePacketEncoder(byte(f.Type()))
	// message
	msgBlock := y3.NewPrimitivePacketEncoder(byte(TagOfRejectedMessage))
	msgBlock.SetStringValue(f.message)

	rejected.AddPrimitivePacket(msgBlock)

	return rejected.Encode()
}

// Message rejected message
func (f *RejectedFrame) Message() string {
	return f.message
}

// DecodeToRejectedFrame decodes Y3 encoded bytes to RejectedFrame
func DecodeToRejectedFrame(buf []byte) (*RejectedFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}
	rejected := &RejectedFrame{}
	// message
	if msgBlock, ok := node.PrimitivePackets[byte(TagOfRejectedMessage)]; ok {
		msg, err := msgBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		rejected.message = msg
	}
	return rejected, nil
}
