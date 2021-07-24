package framing

import (
	"errors"
)

// Frame represents a YoMo frame.
//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//  |                    Frame Length               |
//  +-----------------------------------------------+
//  |    Header   |           Data                  |
//  |                                               |
//  +-----------------------------------------------+
type Frame interface {
	// Bytes converts the frame to bytes.
	Bytes() []byte

	// Type gets the type of frame.
	Type() FrameType

	// Data gets the data in frame.
	Data() []byte
}

// FrameType is type of frame.
type FrameType uint8

const (
	FrameLengthFieldSize = 3 // FrameLengthFieldSize is the size of FrameLength.

	FrameTypeHandshake    FrameType = 0x00 // FrameTypeHandshake represents the frame type Handshake.
	FrameTypeHeartbeat    FrameType = 0x01 // FrameTypeHandshake represents the frame type Heartbeat.
	FrameTypeAck          FrameType = 0x02 // FrameTypeHandshake represents the frame type ACK.
	FrameTypeAccepted     FrameType = 0x03 // FrameTypeHandshake represents the frame type Accepted.
	FrameTypeRejected     FrameType = 0x04 // FrameTypeHandshake represents the frame type Rejected.
	FrameTypeCreateStream FrameType = 0x05 // FrameTypeHandshake represents the frame type CreateStream.
	FrameTypePayload      FrameType = 0x06 // FrameTypeHandshake represents the frame type Payload.
	FrameTypeInit         FrameType = 0x07 // FrameTypeHandshake represents the frame type Init.
)

// frame is an implementation of Frame.
type frame struct {
	header *Header
	data   []byte
}

func newFrame(frameType FrameType) *frame {
	return &frame{
		header: newHeader(frameType),
	}
}

func newFrameWithData(frameType FrameType, data []byte) *frame {
	return &frame{
		header: newHeader(frameType),
		data:   data,
	}
}

// Bytes get the bytes of PayloadFrame.
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

// Data gets the data in frame.
func (f *frame) Data() []byte {
	return f.data
}

func (f *frame) getFrameLengthBytes() []byte {
	buf := make([]byte, FrameLengthFieldSize)
	len := f.header.len() + len(f.data)

	// set len to buf.
	for i := 0; i < FrameLengthFieldSize; i++ {
		offset := 8 * (FrameLengthFieldSize - i - 1)
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
		data:   buf[FrameHeaderSize:],
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

	return c, buf
}

// GetRawBytesWithoutFraming gets the raw bytes without framing bytes.
func GetRawBytesWithoutFraming(buf []byte) []byte {
	headLen := FrameLengthFieldSize + FrameHeaderSize
	if len(buf) <= headLen {
		return buf
	}

	return buf[headLen:]
}
