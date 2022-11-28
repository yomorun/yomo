package frame

type AckFrame struct{}

func NewAckFrame() *AckFrame {
	return &AckFrame{}
}

// Type gets the type of Frame.
func (f *AckFrame) Type() Type {
	return TagOfAckFrame
}

func (f *AckFrame) Encode() []byte {
	return []byte{}
}

func DecodeToAckFrame(buf []byte) (*AckFrame, error) { return &AckFrame{}, nil }
