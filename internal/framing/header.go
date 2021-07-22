package framing

import "errors"

// Header is the header of frame.
type Header struct {
	// Type is the type of frame.
	FrameType FrameType
}

// FrameHeaderSize is the size of FrameHeader.
const FrameHeaderSize = 1

// newHeader inits a new frame handler.
func newHeader(frameType FrameType) *Header {
	return &Header{
		FrameType: frameType,
	}
}

// Bytes gets the bytes of frame header.
func (h *Header) Bytes() []byte {
	return []byte{
		byte(h.FrameType),
	}
}

// len gets the lenght of frame header.
func (h *Header) len() int {
	return FrameHeaderSize
}

// HeaderFromBytes create a new frame header from bytes.
func HeaderFromBytes(buf []byte) (*Header, error) {
	if len(buf) < FrameHeaderSize {
		return nil, errors.New("header: incomplete frame")
	}

	frameType := FrameType(buf[0])
	return newHeader(frameType), nil
}
