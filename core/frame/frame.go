// Package frame defines frames for yomo.
package frame

import (
	"context"
	"fmt"
	"io"
	"net"
)

// Frame is the minimum unit required for Yomo to run.
// Yomo transmits various instructions and data through the frames.
//
// following frame types are supported by Yomo:
//  1. HandshakeFrame
//  2. HandshakeAckFrame
//  3. DataFrame
//  4. BackflowFrame
//  5. RejectedFrame
//  6. GoawayFrame
//
// Read frame comments to understand the role of the frame.
type Frame interface {
	// Type returns the type of frame.
	Type() Type
}

// Type defined The type of frame.
type Type byte

// DataFrame carries tagged data to transmit across connection.
type DataFrame struct {
	// Metadata stores additional data beyond the Payload,
	// it is an map[string]string{} that be encoded in msgpack.
	Metadata []byte
	// Tag is used for data router.
	Tag Tag
	// Payload is the data to transmit.
	Payload []byte
}

// Type returns the type of DataFrame.
func (f *DataFrame) Type() Type { return TypeDataFrame }

// The HandshakeFrame is the frame through which the client obtains a new connection from the server.
// It includes essential details required for the creation of a fresh connection.
// The server then generates the connection utilizing this provided information.
type HandshakeFrame struct {
	// Name is the name of the dataStream that will be created.
	Name string
	// ID is the ID of the dataStream that will be created.
	ID string
	// ClientType is the type of client.
	ClientType byte
	// ObserveDataTags is the ObserveDataTags of the dataStream that will be created.
	ObserveDataTags []Tag
	// AuthName is the authentication name.
	AuthName string
	// AuthPayload is the authentication payload.
	AuthPayload string
}

// Type returns the type of HandshakeFrame.
func (f *HandshakeFrame) Type() Type { return TypeHandshakeFrame }

// HandshakeAckFrame is used to ack handshake, If handshake successful, The server will
// send HandshakeAckFrame to the client.
type HandshakeAckFrame struct{}

// Type returns the type of HandshakeAckFrame.
func (f *HandshakeAckFrame) Type() Type { return TypeHandshakeAckFrame }

// The BackflowFrame is used to receive the processed result of a connection with StreamFunction type
// and forward it to a connection with StreamSource type.
type BackflowFrame struct {
	// Tag is used for data router.
	Tag Tag
	// Carriage is the data to transmit.
	Carriage []byte
}

// Type returns the type of BackflowFrame.
func (f *BackflowFrame) Type() Type { return TypeBackflowFrame }

// RejectedFrame is used to reject a client request.
type RejectedFrame struct {
	// Message encapsulates the rationale behind the rejection of the request.
	Message string
}

// Type returns the type of RejectedFrame.
func (f *RejectedFrame) Type() Type { return TypeRejectedFrame }

// GoawayFrame is is used by server to evict a connection.
type GoawayFrame struct {
	// Message contains the reason why the connection be evicted.
	Message string
}

// Type returns the type of GoawayFrame.
func (f *GoawayFrame) Type() Type { return TypeGoawayFrame }

const (
	TypeDataFrame         Type = 0x3F // TypeDataFrame is the type of DataFrame.
	TypeHandshakeFrame    Type = 0x31 // TypeHandshakeFrame is the type of HandshakeFrame.
	TypeHandshakeAckFrame Type = 0x29 // TypeHandshakeAckFrame is the type of HandshakeAckFrame.
	TypeRejectedFrame     Type = 0x39 // TypeRejectedFrame is the type of RejectedFrame.
	TypeBackflowFrame     Type = 0x2D // TypeBackflowFrame is the type of BackflowFrame.
	TypeGoawayFrame       Type = 0x2E // TypeGoawayFrame is the type of GoawayFrame.
)

var frameTypeStringMap = map[Type]string{
	TypeDataFrame:         "DataFrame",
	TypeHandshakeFrame:    "HandshakeFrame",
	TypeHandshakeAckFrame: "HandshakeAckFrame",
	TypeRejectedFrame:     "RejectedFrame",
	TypeBackflowFrame:     "BackflowFrame",
	TypeGoawayFrame:       "GoawayFrame",
}

// String returns a human-readable string which represents the frame type.
// The string can be used for debugging or logging purposes.
func (f Type) String() string {
	frameString, ok := frameTypeStringMap[f]
	if ok {
		return frameString
	}
	return "UnknownFrame"
}

var frameTypeNewFuncMap = map[Type]func() Frame{
	TypeDataFrame:         func() Frame { return new(DataFrame) },
	TypeHandshakeFrame:    func() Frame { return new(HandshakeFrame) },
	TypeHandshakeAckFrame: func() Frame { return new(HandshakeAckFrame) },
	TypeRejectedFrame:     func() Frame { return new(RejectedFrame) },
	TypeBackflowFrame:     func() Frame { return new(BackflowFrame) },
	TypeGoawayFrame:       func() Frame { return new(GoawayFrame) },
}

// NewFrame creates a new frame from Type.
func NewFrame(f Type) (Frame, error) {
	newFunc, ok := frameTypeNewFuncMap[f]
	if ok {
		return newFunc(), nil
	}
	return nil, fmt.Errorf("frame: cannot new a frame from %c", f)
}

// PacketReadWriter reads packets from the io.Reader and writes packets to the io.Writer.
// If read failed, return the frameType, the data of the packet and an error.
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

// Tag tags data and can be used for data routing.
type Tag = uint32

// Writer is the interface that wraps the WriteFrame method, it writes
// frame to the underlying connection.
type Writer interface {
	// WriteFrame writes frame to underlying connection.
	WriteFrame(Frame) error
}

// Listener accepts Conns.
type Listener interface {
	// Accept accepts Conns.
	Accept(context.Context) (Conn, error)
	// Close closes listener,
	// If listener be closed, all Conn accepted will be unavailable.
	Close() error
}

// Conn is a connection that transmits data in frame format.
type Conn interface {
	// Context returns Conn.Context.
	// The Context can be used to manage the lifecycle of connection and
	// retrieve error using `context.Cause(conn.Context())` after calling `CloseWithError()`.
	Context() context.Context
	// WriteFrame writes a frame to connection.
	WriteFrame(Frame) error
	// ReadFrame returns a channel from which frames can be received.
	ReadFrame() (Frame, error)
	// RemoteAddr returns the remote address of connection.
	RemoteAddr() net.Addr
	// LocalAddr returns the local address of connection.
	LocalAddr() net.Addr
	// CloseWithError closes the connection with an error message.
	// It will be unavailable if the connection is closed. the error message should be written to the conn.Context().
	CloseWithError(string) error
}

// ErrConnClosed is returned when the connection be closed by remote or local.
// The ReadFrame() and WriteFrame() should return this error after calling CloseWithError().
type ErrConnClosed struct {
	Remote       bool
	ErrorMessage string
}

// Error implements the error interface and returns the reason why the connection was closed.
func (e *ErrConnClosed) Error() string {
	if e.Remote {
		return fmt.Sprintf("remote conn closed: %s", e.ErrorMessage)
	}
	return fmt.Sprintf("local conn closed: %s", e.ErrorMessage)
}

// NewErrConnClosed returns an ErrConnClosed.
func NewErrConnClosed(remote bool, errMsg string) *ErrConnClosed {
	return &ErrConnClosed{
		Remote:       remote,
		ErrorMessage: errMsg,
	}
}
