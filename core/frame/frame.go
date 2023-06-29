// Package frame defines frames for yomo.
package frame

import (
	"fmt"
	"io"
)

// Frame is the minimum unit required for Yomo to run.
// Yomo transmits various instructions and data through the frame, which can be transmitted on the IO stream.
//
//	Yomo needs 9 type frame to run up, them cantain:
//		1. AuthenticationFrame
//		2. AuthenticationAckFrame
//		3. DataFrame
//		4. PayloadFrame
//		5. HandshakeFrame
//		6. HandshakeRejectedFrame
//		7. HandshakeAckFrame
//		8. RejectedFrame
//		9. BackflowFrame
//	 Read frame comments to understand the role of the frame.
//
//		If you want to transmit the frame on the IO stream, you must have `ReadFunc` and `WriteFunc` for reading and writing frames.
type Frame interface {
	// Type returns the type of frame.
	Type() Type
}

// Type defined The type of frame.
type Type byte

// AuthenticationFrame is used to authenticate the client,
// Once the connection is established, the client immediately, sends information to the server,
// server gets the way to authenticate according to AuthName and use AuthPayload to do a authentication.
// AuthenticationFrame is transmit on ControlStream.
//
// Reading the `auth.Authentication` interface will help you understand how AuthName and AuthPayload work.
type AuthenticationFrame struct {
	// AuthName.
	AuthName string
	// AuthPayload.
	AuthPayload string
}

// Type returns the type of AuthenticationFrame.
func (f *AuthenticationFrame) Type() Type { return TypeAuthenticationFrame }

// AuthenticationAckFrame is used to confirm that the client is authorized to access the requested DataStream from
// ControlStream, AuthenticationAckFrame is transmit on ControlStream.
// If the client-side receives this frame, it indicates that authentication was successful.
type AuthenticationAckFrame struct{}

// Type returns the type of AuthenticationAckFrame.
func (f *AuthenticationAckFrame) Type() Type { return TypeAuthenticationAckFrame }

// DataFrame carrys taged data to transmit accross DataStream.
type DataFrame struct {
	// Meta.
	Meta *MetaFrame
	// Payload.
	Payload *PayloadFrame
}

// Type returns the type of DataFrame.
func (f *DataFrame) Type() Type { return TypeDataFrame }

// MetaFrame is used to describe a DataFrame, It is a part of DataFrame.
type MetaFrame struct {
	// TID trace a DataFrame.
	TID string
	// Metadata stores additional data beyond the Payload.
	Metadata []byte
	// SourceID records who sent this DataFrame.
	SourceID string
	// Broadcast indicates that this DataFrame should be broadcast to cascading mesh nodes.
	Broadcast bool
}

// Type returns the type of MetaFrame.
func (f *MetaFrame) Type() Type { return TypePayloadFrame }

// PayloadFrame is used to carry taged data for DataFrame. It is a part of DataFrame.
type PayloadFrame struct {
	// Tag is used for data router.
	Tag Tag
	// Carriage is the data to transmit.
	Carriage []byte
}

// Type returns the type of PayloadFrame.
func (f *PayloadFrame) Type() Type { return TypePayloadFrame }

// HandshakeFrame is the frame that client accquires new dataStream from server,
// It includes some of the information necessary to create a new DataStream.
// The server creates DataStream based on this information.
type HandshakeFrame struct {
	// Name is the name of the dataStream that will be created.
	Name string
	// ID is the ID of the dataStream that will be created.
	ID string
	// StreamType is the StreamType of the dataStream that will be created.
	StreamType byte
	// ObserveDataTags is the ObserveDataTags of the dataStream that will be created.
	ObserveDataTags []Tag
	// Metadata is the Metadata of the dataStream that will be created.
	Metadata []byte
}

// Type returns the type of HandshakeFrame.
func (f *HandshakeFrame) Type() Type { return TypeHandshakeFrame }

// HandshakeAckFrame is used to ack handshake, If handshake successful, The server will
// send HandshakeAckFrame to the new DataStream, That means the new DataStream receive first frame
// must be HandshakeAckFrame.
type HandshakeAckFrame struct {
	StreamID string
}

// Type returns the type of HandshakeAckFrame.
func (f *HandshakeAckFrame) Type() Type { return TypeHandshakeAckFrame }

// HandshakeRejectedFrame be used to reject a Handshake. It transmits on ControlStream.
type HandshakeRejectedFrame struct {
	// ID is the ID of DataStream be rejected.
	ID string
	// Message contains the reason why the handshake was not successful.
	Message string
}

// Type returns the type of HandshakeRejectedFrame.
func (f *HandshakeRejectedFrame) Type() Type { return TypeHandshakeRejectedFrame }

// The BackflowFrame is used to receive the processed result of a DataStream with StreamFunction type
// and forward it to a DataStream with StreamSource type.
type BackflowFrame struct {
	// Tag is used for data router.
	Tag Tag
	// Carriage is the data to transmit.
	Carriage []byte
}

// Type returns the type of BackflowFrame.
func (f *BackflowFrame) Type() Type { return TypeBackflowFrame }

// RejectedFrame is is used to reject a ControlStream reqeust.
type RejectedFrame struct {
	// Code is the code rejected.
	Code uint64
	// Message contains the reason why the reqeust be rejected.
	Message string
}

// Type returns the type of RejectedFrame.
func (f *RejectedFrame) Type() Type { return TypeRejectedFrame }

const (
	TypeAuthenticationFrame    Type = 0x03 // TypeAuthenticationFrame is the type of AuthenticationFrame.
	TypeAuthenticationAckFrame Type = 0x11 // TypeAuthenticationAckFrame is the type of AuthenticationAckFrame.
	TypeDataFrame              Type = 0x3F // TypeDataFrame is the type of DataFrame.
	TypePayloadFrame           Type = 0x2E // TypePayloadFrame is the type of PayloadFrame.
	TypeHandshakeFrame         Type = 0x31 // TypeHandshakeFrame is the type of PayloadFrame.
	TypeHandshakeRejectedFrame Type = 0x14 // TypeHandshakeRejectedFrame is the type of HandshakeRejectedFrame.
	TypeHandshakeAckFrame      Type = 0x29 // TypeHandshakeAckFrame is the type of HandshakeAckFrame.
	TypeRejectedFrame          Type = 0x39 // TypeRejectedFrame is the type of RejectedFrame.
	TypeBackflowFrame          Type = 0x2D // TypeBackflowFrame is the type of BackflowFrame.
)

var frameTypeStringMap = map[Type]string{
	TypeAuthenticationFrame:    "AuthenticationFrame",
	TypeAuthenticationAckFrame: "AuthenticationAckFrame",
	TypeDataFrame:              "DataFrame",
	TypePayloadFrame:           "PayloadFrame",
	TypeHandshakeFrame:         "HandshakeFrame",
	TypeHandshakeRejectedFrame: "HandshakeRejectedFrame",
	TypeHandshakeAckFrame:      "HandshakeAckFrame",
	TypeRejectedFrame:          "RejectedFrame",
	TypeBackflowFrame:          "BackflowFrame",
}

// String returns a human-readable string which represents the frame type.
// The string can be used for debugging or logging purposes.
func (f Type) String() string {
	frameString, ok := frameTypeStringMap[f]
	if ok {
		return frameString
	}
	return "UnkonwnFrame"
}

var frameTypeNewFuncMap = map[Type]func() Frame{
	TypeAuthenticationFrame:    func() Frame { return new(AuthenticationFrame) },
	TypeAuthenticationAckFrame: func() Frame { return new(AuthenticationAckFrame) },
	TypeDataFrame:              func() Frame { return &DataFrame{Meta: new(MetaFrame), Payload: new(PayloadFrame)} },
	TypePayloadFrame:           func() Frame { return new(PayloadFrame) },
	TypeHandshakeFrame:         func() Frame { return new(HandshakeFrame) },
	TypeHandshakeRejectedFrame: func() Frame { return new(HandshakeAckFrame) },
	TypeHandshakeAckFrame:      func() Frame { return new(HandshakeAckFrame) },
	TypeRejectedFrame:          func() Frame { return new(RejectedFrame) },
	TypeBackflowFrame:          func() Frame { return new(BackflowFrame) },
}

// NewFrame creates a new frame from Type.
func NewFrame(f Type) (Frame, error) {
	newFunc, ok := frameTypeNewFuncMap[f]
	if ok {
		return newFunc(), nil
	}
	return nil, fmt.Errorf("frame: cannot new a frame from %c", f)
}

// PacketReadWriter reads packet from the io.Reader and writes packet to the io.Writer.
// It returns frameType, the data of the packet and an error if read failed.
type PacketReadWriter interface {
	ReadPacket(io.Reader) (Type, []byte, error)
	WritePacket(io.Writer, Type, []byte) error
}

// Codec encodes and decodes byte array to frame.
type Codec interface {
	// Decode decodes byte array to frame.
	Decode([]byte, Frame) error
	// Encode encodes frame to byte array.
	Encode(Frame) ([]byte, error)
}

// Tag tags data and be used for data routing.
type Tag = uint32

// ReadWriteCloser is the interface that groups the ReadFrame, WriteFrame and Close methods.
type ReadWriteCloser interface {
	Reader
	Writer
	Close() error
}

// ReadWriter is the interface that groups the ReadFrame and WriteFrame methods.
type ReadWriter interface {
	Reader
	Writer
}

// Writer is the interface that wraps the WriteFrame method, It writes
// frame to the underlying data stream.
type Writer interface {
	// WriteFrame writes frame to underlying stream.
	WriteFrame(Frame) error
}

// Reader reads frame from underlying stream.
type Reader interface {
	// ReadFrame reads frame, if error, the error returned is not empty
	// and frame returned is nil.
	ReadFrame() (Frame, error)
}
