package core

import (
	"fmt"
	"io"

	"github.com/yomorun/y3"
	"github.com/yomorun/yomo/core/frame"
)

// ParseFrame parses the frame from QUIC stream.
func ParseFrame(stream io.Reader) (frame.Frame, error) {
	buf, err := y3.ReadPacket(stream)
	if err != nil {
		return nil, err
	}
	// if len(buf) > 512 {
	// 	logger.Debugf("%s🔗 parsed out total %d bytes: \n\thead 64 bytes are: [%# x], \n\ttail 64 bytes are: [%#x]", ParseFrameLogPrefix, len(buf), buf[0:64], buf[len(buf)-64:])
	// } else {
	// 	logger.Debugf("%s🔗 parsed out: [%# x]", ParseFrameLogPrefix, buf)
	// }

	frameType := buf[0]
	// determine the frame type
	switch frameType {
	case 0x80 | byte(frame.TagOfHandshakeFrame):
		handshakeFrame, err := readHandshakeFrame(buf)
		// logger.Debugf("%sHandshakeFrame: name=%s, type=%s", ParseFrameLogPrefix, handshakeFrame.Name, handshakeFrame.Type())
		return handshakeFrame, err
	case 0x80 | byte(frame.TagOfDataFrame):
		data, err := readDataFrame(buf)
		// logger.Debugf("%sDataFrame: tid=%s, tag=%#x, len(carriage)=%d", ParseFrameLogPrefix, data.TransactionID(), data.GetDataTag(), len(data.GetCarriage()))
		return data, err
	case 0x80 | byte(frame.TagOfAcceptedFrame):
		return frame.DecodeToAcceptedFrame(buf)
	case 0x80 | byte(frame.TagOfRejectedFrame):
		return frame.DecodeToRejectedFrame(buf)
	case 0x80 | byte(frame.TagOfGoawayFrame):
		return frame.DecodeToGoawayFrame(buf)
	case 0x80 | byte(frame.TagOfBackflowFrame):
		return frame.DecodeToBackflowFrame(buf)
	default:
		// todo: distinguish between illegal protocol and newer unknown frame from peer side
		return nil, fmt.Errorf("unknown frame type, buf[0]=%#x", buf[0])
	}
}

func readHandshakeFrame(buf []byte) (*frame.HandshakeFrame, error) {
	// parse to HandshakeFrame
	// handshake, err := frame.DecodeToHandshakeFrame(buf)
	// if err != nil {
	// 	logger.Errorf("%sreadHandshakeFrame: err=%v", ParseFrameLogPrefix, err)
	// 	return nil
	// }
	// return handshake
	return frame.DecodeToHandshakeFrame(buf)
}

func readDataFrame(buf []byte) (*frame.DataFrame, error) {
	// parse to DataFrame
	// data, err := frame.DecodeToDataFrame(buf)
	// if err != nil {
	// 	logger.Errorf("%sreadDataFrame: err=%v", ParseFrameLogPrefix, err)
	// 	return err
	// }
	// return data
	return frame.DecodeToDataFrame(buf)
}
