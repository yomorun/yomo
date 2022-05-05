package frame

import (
	"github.com/yomorun/y3"
)

// GoawayFrame is a Y3 encoded bytes, Tag is a fixed value TYPE_ID_GOAWAY_FRAME
type GoawayFrame struct {
	message string
}

// NewGoawayFrame creates a new GoawayFrame
func NewGoawayFrame(msg string) *GoawayFrame {
	return &GoawayFrame{message: msg}
}

// Type gets the type of Frame.
func (f *GoawayFrame) Type() Type {
	return TagOfGoawayFrame
}

// Encode to Y3 encoded bytes
func (f *GoawayFrame) Encode() []byte {
	goaway := y3.NewNodePacketEncoder(byte(f.Type()))
	// message
	msgBlock := y3.NewPrimitivePacketEncoder(byte(TagOfGoawayMessage))
	msgBlock.SetStringValue(f.message)

	goaway.AddPrimitivePacket(msgBlock)

	return goaway.Encode()
}

// Message goaway message
func (f *GoawayFrame) Message() string {
	return f.message
}

// DecodeToGoawayFrame decodes Y3 encoded bytes to GoawayFrame
func DecodeToGoawayFrame(buf []byte) (*GoawayFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	goaway := &GoawayFrame{}
	// message
	if msgBlock, ok := node.PrimitivePackets[byte(TagOfGoawayMessage)]; ok {
		msg, err := msgBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		goaway.message = msg
	}
	return goaway, nil
}
