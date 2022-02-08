package frame

// Kinds of frames transferable within YoMo
const (
	// MetaFrame
	TagOfTimestamp    Type = 0x01
	TagOfLBType       Type = 0x02
	TagOfToInstanceID Type = 0x03

	// HandshakeFrame
	TagOfHandshakeName        Type = 0x01
	TagOfHandshakeType        Type = 0x02
	TagOfHandshakeAppID       Type = 0x03
	TagOfHandshakeAuthType    Type = 0x04
	TagOfHandshakeAuthPayload Type = 0x05
	TagOfHandshakeInstanceID  Type = 0x06

	// Frame types
	TagOfHandshakeFrame Type = 0x3D
	TagOfMetaFrame      Type = 0x2F
	TagOfPayloadFrame   Type = 0x2E
	TagOfDataFrame      Type = 0x3F
	TagOfTokenFrame     Type = 0x3E
	TagOfPingFrame      Type = 0x3C
	TagOfPongFrame      Type = 0x3B
	TagOfAcceptedFrame  Type = 0x3A
	TagOfRejectedFrame  Type = 0x39
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
	case TagOfMetaFrame:
		return "MetaFrame"
	case TagOfPayloadFrame:
		return "PayloadFrame"
	case TagOfHandshakeName:
		return "HandshakeName"
	case TagOfHandshakeType:
		return "HandshakeType"
	default:
		return "UnknownFrame"
	}
}
