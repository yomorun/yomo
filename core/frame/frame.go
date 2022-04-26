package frame

import (
	"os"
	"strconv"
)

// debugFrameSize print frame data size on debug mode
var debugFrameSize = 16

// Kinds of frames transferable within YoMo
const (
	// DataFrame
	TagOfDataFrame Type = 0x3F
	// MetaFrame of DataFrame
	TagOfMetaFrame     Type = 0x2F
	TagOfMetadata      Type = 0x03
	TagOfTransactionID Type = 0x01
	// PayloadFrame of DataFrame
	TagOfPayloadFrame  Type = 0x2E
	TagOfBackflowFrame Type = 0x2D

	TagOfTokenFrame Type = 0x3E
	// HandshakeFrame
	TagOfHandshakeFrame Type = 0x3D
	TagOfHandshakeName  Type = 0x01
	TagOfHandshakeType  Type = 0x02
	// TagOfHandshakeAppID           Type = 0x03
	// TagOfHandshakeAuthType        Type = 0x04
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
	default:
		return "UnknownFrame"
	}
}

// Shortly reduce data size for easy viewing
func Shortly(data []byte) []byte {
	if len(data) > debugFrameSize {
		return data[:debugFrameSize]
	}
	return data
}

func init() {
	if envFrameSize := os.Getenv("YOMO_DEBUG_FRAME_SIZE"); envFrameSize != "" {
		if val, err := strconv.Atoi(envFrameSize); err == nil {
			debugFrameSize = val
		}
	}
}
