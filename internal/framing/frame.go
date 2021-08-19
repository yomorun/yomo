package framing

import (
	"errors"
)

// Frame represents a YoMo frame.
//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//  |                  Frame Length                 |
//  +-----------------------------------------------+
//  |                     Header                    |
//  +-----------------------------------------------+
//  |                      Data                     |
//  +-----------------------------------------------+
type Frame interface {
	// Bytes converts the frame to bytes.
	Bytes() []byte

	// Type gets the type of frame.
	Type() FrameType

	// Metadata gets the metadata in frame.
	Metadata() []byte

	// Data gets the data in frame.
	Data() []byte
}

// FrameType is type of frame.
type FrameType uint8

const (
	// FrameLengthFieldSize is the size of FrameLength in bytes.
	FrameLengthFieldSize = 3
	// FrameTypeHandshake is the frame type HANDSHAKE.
	FrameTypeHandshake FrameType = 0x00
	// FrameTypeHeartbeat is the frame type HEARTBEAT.
	FrameTypeHeartbeat FrameType = 0x01
	// FrameTypeAck is the frame type ACK.
	FrameTypeAck FrameType = 0x02
	// FrameTypeAccepted is the frame type ACCEPTED.
	FrameTypeAccepted FrameType = 0x03
	// FrameTypeRejected is the frame type REJECTED.
	FrameTypeRejected FrameType = 0x04
	// FrameTypeCreateStream is the frame type CREATE_STREAM.
	FrameTypeCreateStream FrameType = 0x05
	// FrameTypePayload is the frame type PAYLOAD.
	FrameTypePayload FrameType = 0x06
	// FrameTypeInit is the frame type INIT.
	FrameTypeInit FrameType = 0x07
)

// frame is an implementation of Frame.
type frame struct {
	header *Header
	data   []byte
}

func newFrame(frameType FrameType, opts ...Option) *frame {
	options := newOptions(opts...)
	return &frame{
		header: newHeader(frameType, options.Metadata),
	}
}

func newFrameWithData(frameType FrameType, data []byte, opts ...Option) *frame {
	options := newOptions(opts...)
	return &frame{
		header: newHeader(frameType, options.Metadata),
		data:   data,
	}
}

// Bytes get the bytes of frame.
func (f *frame) Bytes() []byte {
	buf := f.getFrameLengthBytes()
	buf = append(buf, f.header.Bytes()...)
	buf = append(buf, f.data...)
	return buf
}

// Type gets the type of frame.
func (f *frame) Type() FrameType {
	return f.header.FrameType
}

// Metadata gets the metadata of frame.
func (f *frame) Metadata() []byte {
	return f.header.Metadata
}

// Data gets the data in frame.
func (f *frame) Data() []byte {
	return f.data
}

func (f *frame) getFrameLengthBytes() []byte {
	len := f.header.len() + len(f.data)

	return getLengthInBytes(FrameLengthFieldSize, len)
}

// getLengthBytes gets the length in bytes.
func getLengthInBytes(sizeOfBytes int, len int) []byte {
	buf := make([]byte, sizeOfBytes)

	// set len to buf.
	for i := 0; i < sizeOfBytes; i++ {
		offset := 8 * (sizeOfBytes - i - 1)
		if offset > 0 {
			buf[i] = byte(len >> offset)
		} else {
			buf[i] = byte(len)
		}
	}
	return buf
}

// FromRawBytes creates a frame from raw bytes.
func FromRawBytes(buf []byte) (Frame, error) {
	header, err := HeaderFromBytes(buf)
	if err != nil {
		return nil, err
	}

	f := &frame{
		header: header,
		data:   buf[header.len():],
	}

	return convertSpecificFrame(f)
}

// convertSpecificFrame converts the frames to a specific frame.
func convertSpecificFrame(f *frame) (Frame, error) {
	switch f.header.FrameType {
	case FrameTypeHandshake:
		return &HandshakeFrame{
			frame: f,
		}, nil
	case FrameTypeHeartbeat:
		return &HeartbeatFrame{
			frame: f,
		}, nil
	case FrameTypeAck:
		return &AckFrame{
			frame: f,
		}, nil
	case FrameTypeAccepted:
		return &AcceptedFrame{
			frame: f,
		}, nil
	case FrameTypeRejected:
		return &RejectedFrame{
			frame: f,
		}, nil
	case FrameTypeCreateStream:
		return &CreateStreamFrame{
			frame: f,
		}, nil
	case FrameTypePayload:
		return &PayloadFrame{
			frame: f,
		}, nil
	case FrameTypeInit:
		return &InitFrame{
			frame: f,
		}, nil
	default:
		return nil, errors.New("invalid frame type")
	}
}

// ReadFrameLength reads frame length from bytes.
func ReadFrameLength(buf []byte) int {
	return readLengthFromBytes(buf, FrameLengthFieldSize)
}

// readLengthFromBytes reads length from bytes.
func readLengthFromBytes(buf []byte, sizeOfLen int) int {
	if len(buf) < sizeOfLen {
		return 0
	}

	c := 0
	for i := 0; i < sizeOfLen; i++ {
		offset := 8 * (sizeOfLen - i - 1)
		if offset > 0 {
			c += int(buf[i]) << offset
		} else {
			c += int(buf[i])
		}
	}

	return c
}
