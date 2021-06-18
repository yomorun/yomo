package framing

// PayloadFrame represents a payload frame.
type PayloadFrame struct {
	data []byte
}

// NewPayloadFrame inits a new PayloadFrame.
func NewPayloadFrame(data []byte) *PayloadFrame {
	return &PayloadFrame{
		data: data,
	}
}

func (p *PayloadFrame) Bytes() []byte {
	len := len(p.data)
	buf := make([]byte, FrameLengthFieldSize)

	appendFrameLength(buf, len)
	buf = append(buf, p.data...)
	return buf
}
