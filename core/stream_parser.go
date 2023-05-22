package core

import (
	"fmt"
	"io"

	"github.com/yomorun/y3"
	"github.com/yomorun/yomo/core/frame"
)

// FrameReadFunc is the function to read frames from stream.
// FrameReadFunc reads bytes from the input stream, parses them into a frame, and returns the parsed result.
// This function should be called inside a for-loop to read and parse each frame sequentially.
// Parse can return two types of errors: a parse error if there is an error while parsing the stream data,
// and a stream read error.
type FrameReadFunc func(io.Reader) (frame.Frame, error)

// ParseFrame parses the frame from QUIC stream.
func ParseFrame(stream io.Reader) (frame.Frame, error) {
	buf, err := y3.ReadPacket(stream)
	if err != nil {
		return nil, err
	}

	frameType := buf[0]
	switch frameType {
	case 0x80 | byte(frame.TagOfHandshakeFrame):
		return frame.DecodeToHandshakeFrame(buf)
	case 0x80 | byte(frame.TagOfDataFrame):
		return frame.DecodeToDataFrame(buf)
	case 0x80 | byte(frame.TagOfAcceptedFrame):
		return frame.DecodeToAcceptedFrame(buf)
	case 0x80 | byte(frame.TagOfRejectedFrame):
		return frame.DecodeToRejectedFrame(buf)
	case 0x80 | byte(frame.TagOfGoawayFrame):
		return frame.DecodeToGoawayFrame(buf)
	case 0x80 | byte(frame.TagOfBackflowFrame):
		return frame.DecodeToBackflowFrame(buf)
	case 0x80 | byte(frame.TagOfHandshakeAckFrame):
		return frame.DecodeToHandshakeAckFrame(buf)
	case 0x80 | byte(frame.TagOfAuthenticationFrame):
		return frame.DecodeToAuthenticationFrame(buf)
	case 0x80 | byte(frame.TagOfHandshakeRejectedFrame):
		return frame.DecodeToHandshakeRejectedFrame(buf)
	case 0x80 | byte(frame.TagOfAuthenticationAckFrame):
		return frame.DecodeToAuthenticationAckFrame(buf)
	default:
		return nil, fmt.Errorf("unknown frame type, buf[0]=%#x", buf[0])
	}
}
