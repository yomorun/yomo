package frame

// Kinds of frames transferable within YoMo
const (
	TagOfDataFrame      FrameType = 0x3F
	TagOfTokenFrame     FrameType = 0x3E
	TagOfHandshakeFrame FrameType = 0x3D
	TagOfPingFrame      FrameType = 0x3C
	TagOfPongFrame      FrameType = 0x3B
	TagOfAcceptedFrame  FrameType = 0x3A
	TagOfRejectedFrame  FrameType = 0x39
	TagOfMetaFrame      FrameType = 0x2F // in `DataFrame`
	TagOfPayloadFrame   FrameType = 0x2E // in `DataFrame`
	TagOfTransactionID  FrameType = 0x01 // in `MetaFrame`
	TagOfHandshakeName  FrameType = 0x01 // in `HandshakeFrame`
	TagOfHandshakeType  FrameType = 0x02 // in `HandshakeFrame`
)

// FrameType defines the type of frame
type FrameType byte

// Frame is the minimal unit transmitted within YoMo
type Frame interface {
	// Type gets the type of Frame.
	Type() FrameType

	// Encode the frame into []byte.
	Encode() []byte
}
