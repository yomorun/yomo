package framing

// Frame represents a YoMo frame.
//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//  |                    Frame Length               |
//  +-----------------------------------------------+
//  |                     Frame                     |
//  |                                               |
//  +-----------------------------------------------+
type Frame interface {
	// Bytes converts the frame to bytes
	Bytes() []byte
}

// FrameLengthFieldSize is the size of FrameLength.
const FrameLengthFieldSize = 3

func appendFrameLength(buf []byte, len int) {
	for i := 0; i < FrameLengthFieldSize; i++ {
		offset := 8 * (FrameLengthFieldSize - i - 1)
		if offset > 0 {
			buf[i] = byte(len >> offset)
		} else {
			buf[i] = byte(len)
		}
	}
}

// ReadFrameLength reads frame length from bytes and returns the clean buf.
func ReadFrameLength(buf []byte) (int, []byte) {
	c := 0
	for i := 0; i < FrameLengthFieldSize; i++ {
		offset := 8 * (FrameLengthFieldSize - i - 1)
		if offset > 0 {
			c += int(buf[i]) << offset
		} else {
			c += int(buf[i])
		}
	}

	if c == 0 && len(buf) > FrameLengthFieldSize {
		// skip the first 0 byte and contine reading frame length from buf
		buf = buf[1:]
		return ReadFrameLength(buf)
	}

	return c, buf
}
