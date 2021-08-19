package parser

import (
	"errors"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/y3"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/internal/utils"
)

var logger = utils.DefaultLogger.WithPrefix("\033[36m[yomo:parser]\033[0m")

// ParseFrame parses the frames from QUIC
func ParseFrame(stream quic.Stream) (frame.Frame, error) {
	buf, err := y3.ReadPacket(stream)
	if err != nil {
		logger.Debugf("\t read first byte err=%v", err)
		return nil, err
	}
	logger.Debugf("parsed out: [%# x]", buf)

	frameType := buf[0]
	// determine the frame type
	switch frameType {
	case 0x80 | byte(frame.TagOfHandshakeFrame):
		handshake := readHandshakeFrame(buf)
		logger.Debugf("[HandshakeFrame] type=%s, name=%s", handshake.ClientType, handshake.Name)
		return handshake, nil
	case 0x80 | byte(frame.TagOfDataFrame):
		data := readDataFrame(buf)
		logger.Debugf("[DataFrame] tid=%s, data-tag=%v", data.TransactionID(), data.GetDataTagID())
		return data, nil
	case 0x80 | byte(frame.TagOfPingFrame):
		return frame.DecodeToPingFrame(buf)
	case 0x80 | byte(frame.TagOfPongFrame):
		return frame.DecodeToPongFrame(buf)
	case 0x80 | byte(frame.TagOfAcceptedFrame):
		return frame.DecodeToAcceptedFrame(buf)
	case 0x80 | byte(frame.TagOfRejectedFrame):
		return frame.DecodeToRejectedFrame(buf)
	default:
		return nil, errors.New("unknown frame type")
	}
}

func readHandshakeFrame(buf []byte) *frame.HandshakeFrame {
	// buf := readY3(reader, 0x80|frame.TYPE_ID_HANDSHAKE_FRMAE)

	// parse to HandshakeFrame
	handshake, err := frame.DecodeToHandshakeFrame(buf)
	if err != nil {
		panic(err)
	}
	return handshake
}

func readDataFrame(buf []byte) *frame.DataFrame {
	// buf := readY3(reader, 0x80|frame.TYPE_ID_DATA_FRAME)

	// parse to HandshakeFrame
	data, err := frame.DecodeToDataFrame(buf)
	if err != nil {
		panic(err)
	}
	return data
}
