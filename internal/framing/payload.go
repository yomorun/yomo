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

// Bytes get the bytes of PayloadFrame.
func (p *PayloadFrame) Bytes() []byte {
	len := len(p.data)

	buf := getFrameLengthBytes(len)
	buf = append(buf, p.data...)
	return buf
}
