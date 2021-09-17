package frame

// Kinds of frames transferable within YoMo
const (
	// DataFrame
	TagOfDataFrame FrameType = 0x3F
	// MetaFrame of DataFrame
	TagOfMetaFrame     FrameType = 0x2F
	TagOfMetadata      FrameType = 0x03
	TagOfTransactionID FrameType = 0x01
	TagOfIssuer        FrameType = 0x02
	// PayloadFrame of DataFrame
	TagOfPayloadFrame FrameType = 0x2E

	TagOfTokenFrame FrameType = 0x3E
	// HandshakeFrame
	TagOfHandshakeFrame FrameType = 0x3D
	TagOfHandshakeName  FrameType = 0x01
	TagOfHandshakeType  FrameType = 0x02

	TagOfPingFrame     FrameType = 0x3C
	TagOfPongFrame     FrameType = 0x3B
	TagOfAcceptedFrame FrameType = 0x3A
	TagOfRejectedFrame FrameType = 0x39
)

// FrameType represents the type of frame.
// TODO: consider change to `Type`
type FrameType uint8

// Frame is the inferface for frame.
type Frame interface {
	// Type gets the type of Frame.
	Type() FrameType

	// Encode the frame into []byte.
	Encode() []byte
}

func (f FrameType) String() string {
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
