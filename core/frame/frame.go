// Package frame defines frames for yomo.
package frame

import (
	"os"
	"strconv"
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

// Writer is the interface that wraps the WriteFrame method, It writes
// frm to the underlying data stream.
type Writer interface {
	WriteFrame(frm Frame) error
}

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

	// AuthenticationFrame
	TagOfAuthenticationFrame   Type = 0x03
	TagOfAuthenticationName    Type = 0x04
	TagOfAuthenticationPayload Type = 0x05

	// AuthenticationRespFrame
	TagOfAuthenticationRespFrame  Type = 0x11
	TagOfAuthenticationRespOk     Type = 0x12
	TagOfAuthenticationRespReason Type = 0x13

	// HandshakeFrame
	TagOfHandshakeFrame           Type = 0x31
	TagOfHandshakeName            Type = 0x01
	TagOfHandshakeStreamType      Type = 0x02
	TagOfHandshakeID              Type = 0x03
	TagOfHandshakeObserveDataTags Type = 0x06
	TagOfHandshakeMetadata        Type = 0x07

	// HandshakeRejectedFrame
	TagOfHandshakeRejectedFrame    Type = 0x14
	TagOfHandshakeRejectedStreamID Type = 0x15
	TagOfHandshakeRejectedReason   Type = 0x16

	// TagOfHandshakeAckFrame
	TagOfHandshakeAckFrame    Type = 0x29
	TagOfHandshakeAckStreamID Type = 0x28

	TagOfPingFrame       Type = 0x3C
	TagOfPongFrame       Type = 0x3B
	TagOfAcceptedFrame   Type = 0x3A
	TagOfRejectedFrame   Type = 0x39
	TagOfRejectedMessage Type = 0x02
	// GoawayFrame
	TagOfGoawayFrame   Type = 0x30
	TagOfGoawayCode    Type = 0x01
	TagOfGoawayMessage Type = 0x02
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
	case TagOfAuthenticationFrame:
		return "AuthenticationFrame"
	case TagOfAuthenticationRespFrame:
		return "AuthenticationRespFrame"
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
	case TagOfHandshakeAckFrame:
		return "HandshakeAckFrame"
	case TagOfHandshakeFrame:
		return "HandshakeFrame"
	case TagOfHandshakeRejectedFrame:
		return "HandshakeRejectFrame"
	default:
		return "UnknownFrame"
	}
}

// debugFrameSize is default to 16,
// if env `YOMO_DEBUG_FRAME_SIZE` is setted and It's an int number, Set the env value to DebugFrameSize.
var debugFrameSize = 16

func init() {
	if e := os.Getenv("YOMO_DEBUG_FRAME_SIZE"); e != "" {
		if val, err := strconv.Atoi(e); err == nil {
			debugFrameSize = val
		}
	}
}
