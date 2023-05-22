package y3codec

import (
	"errors"
	"io"

	"github.com/yomorun/y3"
	"github.com/yomorun/yomo/core/frame"
)

var ErrUnknownFrame = errors.New("y3codec: unknown frame")

type packetReader struct{}

func PacketReader() frame.PacketReader { return &packetReader{} }

func (pr *packetReader) ReadPacket(stream io.Reader) (frame.Type, []byte, error) {
	buf, err := y3.ReadPacket(stream)
	if err != nil {
		return 0, nil, err
	}
	return frame.Type(buf[0] & 0x7F), buf, nil
}

type y3codec struct{}

func Codec() frame.Codec { return &y3codec{} }

func (c *y3codec) Encode(f frame.Frame) ([]byte, error) {
	switch ff := f.(type) {
	case *frame.AuthenticationFrame:
		return encodeAuthenticationFrame(ff)
	case *frame.AuthenticationAckFrame:
		return encodeAuthenticationAckFrame(ff)
	case *frame.RejectedFrame:
		return encodeRejectedFrame(ff)
	case *frame.HandshakeFrame:
		return encodeHandshakeFrame(ff)
	case *frame.HandshakeRejectedFrame:
		return encodeHandshakeRejectedFrame(ff)
	case *frame.HandshakeAckFrame:
		return encodeHandshakeAckFrame(ff)
	case *frame.DataFrame:
		return encodeDataFrame(ff)
	case *frame.BackflowFrame:
		return encodeBackflowFrame(ff)
	default:
		return nil, ErrUnknownFrame
	}
}

func (c *y3codec) Decode(data []byte, f frame.Frame) error {
	switch ff := f.(type) {
	case *frame.AuthenticationFrame:
		return decodeAuthenticationFrame(data, ff)
	case *frame.AuthenticationAckFrame:
		return decodeAuthenticationAckFrame(data, ff)
	case *frame.RejectedFrame:
		return decodeRejectedFrame(data, ff)
	case *frame.HandshakeFrame:
		return decodeHandshakeFrame(data, ff)
	case *frame.HandshakeRejectedFrame:
		return decodeHandshakeRejectedFrame(data, ff)
	case *frame.HandshakeAckFrame:
		return decodeHandshakeAckFrame(data, ff)
	case *frame.DataFrame:
		return decodeDataFrame(data, ff)
	case *frame.BackflowFrame:
		return decodeBackflowFrame(data, ff)
	default:
		return ErrUnknownFrame
	}
}
