package framing

import "errors"

// Header is the header of frame.
//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//  |   Frame Type  |        Metadata Length        |
//  +-----------------------------------------------+
//  |                   Metadata                    |
//  +-----------------------------------------------+
type Header struct {
	// Type is the type of frame.
	FrameType FrameType

	// Metadata describes the additional information of frame.
	Metadata []byte
}

const (
	// FrameTypeSize is the size of FrameType.
	FrameTypeSize = 1

	// FrameTypeSize is the size of Metadata Length.
	MetadataLengthSize = 2
)

// newHeader inits a new frame handler.
func newHeader(frameType FrameType, metadata []byte) *Header {
	return &Header{
		FrameType: frameType,
		Metadata:  metadata,
	}
}

// Bytes gets the bytes of frame header.
func (h *Header) Bytes() []byte {
	metaLen := len(h.Metadata)
	buf := []byte{byte(h.FrameType)}
	buf = append(buf, getLengthBytes(MetadataLengthSize, metaLen)...)
	if metaLen > 0 {
		buf = append(buf, h.Metadata...)
	}
	return buf
}

// len gets the length of frame header.
func (h *Header) len() int {
	return FrameTypeSize + MetadataLengthSize + len(h.Metadata)
}

// HeaderFromBytes create a new frame header from bytes.
func HeaderFromBytes(buf []byte) (*Header, error) {
	if len(buf) < FrameTypeSize+MetadataLengthSize {
		return nil, errors.New("header: incomplete frame")
	}

	frameType := FrameType(buf[0])
	metaLen := readLengthFromBytes(buf[1:MetadataLengthSize], MetadataLengthSize)
	if metaLen > 0 {
		metadata := buf[FrameTypeSize+MetadataLengthSize : metaLen]
		return newHeader(frameType, metadata), nil
	}
	return newHeader(frameType, []byte{}), nil
}
