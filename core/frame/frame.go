package frame

import (
	"errors"
	"time"

	"github.com/yomorun/yomo/core/ylog"
)

// ReadWriter is the interface that groups the ReadFrame and WriteFrame methods.
type ReadWriter interface {
	Reader
	Writer
}

// Reader reads frame from underlying stream.
type Reader interface {
	// ReadFrame reads frame, if error, the error returned is not empty
	// and frame returned is nil.
	ReadFrame() (Frame, error)
}

// ErrReadUntilTimeout be returned when call ReadUntil timeout.
type ErrReadUntilTimeout struct{ t Type }

// Error implement error interface.
func (err ErrReadUntilTimeout) Error() string {
	return "yomo: frame read until timeout, type: " + err.t.String()
}

// ReadUntil reads frame from reader, until the frame of the specified type is returned.
// It returns ErrReadUntilTimeout error if frame not be returned after timeout duration.
// If read a goawayFrame, use goawayFrame.message as error and return it.
func ReadUntil(reader Reader, t Type, timeout time.Duration) (Frame, error) {
	var (
		errch = make(chan error)
		frmch = make(chan Frame)
	)

	go func() {
		for {
			f, err := reader.ReadFrame()
			if err != nil {
				errch <- err
				return
			}
			if f.Type() == TagOfGoawayFrame {
				errch <- errors.New(f.(*GoawayFrame).message)
				return
			}
			if f.Type() == t {
				frmch <- f
				return
			}
		}
	}()

	select {
	case <-time.After(timeout):
		return nil, ErrReadUntilTimeout{t: t}
	case err := <-errch:
		return nil, err
	case frm := <-frmch:
		return frm, nil
	}
}

// Writer is the interface that wraps the WriteFrame method.

// Writer writes Frame from frm to the underlying data stream.
type Writer interface {
	WriteFrame(frm Frame) error
}

// debugFrameSize print frame data size on debug mode
var debugFrameSize = ylog.DebugFrameSize

// Kinds of frames transferable within YoMo
const (
	// DataFrame
	TagOfDataFrame Type = 0x3F
	// MetaFrame of DataFrame
	TagOfMetaFrame     Type = 0x2F
	TagOfMetadata      Type = 0x03
	TagOfTransactionID Type = 0x01
	TagOfSourceID      Type = 0x02
	TagOfBroadcast     Type = 0x04
	// PayloadFrame of DataFrame
	TagOfPayloadFrame     Type = 0x2E
	TagOfPayloadDataTag   Type = 0x01
	TagOfPayloadCarriage  Type = 0x02
	TagOfBackflowFrame    Type = 0x2D
	TagOfBackflowDataTag  Type = 0x01
	TagOfBackflowCarriage Type = 0x02

	TagOfTokenFrame Type = 0x3E
	// HandshakeFrame
	TagOfHandshakeFrame           Type = 0x3D
	TagOfHandshakeName            Type = 0x01
	TagOfHandshakeType            Type = 0x02
	TagOfHandshakeID              Type = 0x03
	TagOfHandshakeAuthName        Type = 0x04
	TagOfHandshakeAuthPayload     Type = 0x05
	TagOfHandshakeObserveDataTags Type = 0x06

	TagOfPingFrame       Type = 0x3C
	TagOfPongFrame       Type = 0x3B
	TagOfAcceptedFrame   Type = 0x3A
	TagOfRejectedFrame   Type = 0x39
	TagOfRejectedMessage Type = 0x02
	// GoawayFrame
	TagOfGoawayFrame   Type = 0x30
	TagOfGoawayCode    Type = 0x01
	TagOfGoawayMessage Type = 0x02
	// TagOfHandshakeAckFrame
	TagOfHandshakeAckFrame Type = 0x29
)

// Type represents the type of frame.
type Type uint8

// Frame is the inferface for frame.
type Frame interface {
	// Type gets the type of Frame.
	Type() Type

	// Encode the frame into []byte.
	Encode() []byte
}

func (f Type) String() string {
	switch f {
	case TagOfDataFrame:
		return "DataFrame"
	case TagOfTokenFrame:
		return "TokenFrame"
	case TagOfHandshakeFrame:
		return "HandshakeFrame"
	case TagOfPingFrame:
		return "PingFrame"
	case TagOfPongFrame:
		return "PongFrame"
	case TagOfAcceptedFrame:
		return "AcceptedFrame"
	case TagOfRejectedFrame:
		return "RejectedFrame"
	case TagOfGoawayFrame:
		return "GoawayFrame"
	case TagOfBackflowFrame:
		return "BackflowFrame"
	case TagOfMetaFrame:
		return "MetaFrame"
	case TagOfPayloadFrame:
		return "PayloadFrame"
	// case TagOfTransactionID:
	// 	return "TransactionID"
	case TagOfHandshakeName:
		return "HandshakeName"
	case TagOfHandshakeType:
		return "HandshakeType"
	case TagOfHandshakeAckFrame:
		return "TagOfHandshakeAckFrame"
	default:
		return "UnknownFrame"
	}
}
