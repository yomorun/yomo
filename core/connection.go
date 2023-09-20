package core

import (
	"context"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// ConnectionInfo holds the information of connection.
type ConnectionInfo interface {
	// Name returns the name of the connection, which is set by clients.
	Name() string
	// ID represents the connection ID, the ID is an unique string.
	ID() string
	// ClientType represents connection type (Source | SFN | UpstreamZipper).
	ClientType() ClientType
	// Metadata returns the extra info of the application.
	Metadata() metadata.M
	// ObserveDataTags observed data tags.
	ObserveDataTags() []frame.Tag
}

// Connection wraps conneciton and stream to transfer frames.
// Connection be used to read and write frames, and be managed by Connector.
type Connection interface {
	Context() context.Context
	ConnectionInfo
	frame.ReadWriteCloser
	// CloseWithError closes the connection with an error string.
	CloseWithError(string) error
}

type connection struct {
	name            string
	id              string
	clientType      ClientType
	metadata        metadata.M
	observeDataTags []uint32
	conn            quic.Connection
	fs              *FrameStream
}

func newConnection(
	name string, id string, clientType ClientType, md metadata.M, tags []uint32,
	conn quic.Connection, fs *FrameStream) *connection {
	return &connection{
		name:            name,
		id:              id,
		clientType:      clientType,
		metadata:        md,
		observeDataTags: tags,
		conn:            conn,
		fs:              fs,
	}
}

func (c *connection) Close() error {
	return c.fs.Close()
}

func (c *connection) Context() context.Context {
	return c.fs.Context()
}

func (c *connection) ID() string {
	return c.id
}

func (c *connection) Metadata() metadata.M {
	return c.metadata
}

func (c *connection) Name() string {
	return c.name
}

func (c *connection) ObserveDataTags() []uint32 {
	return c.observeDataTags
}

func (c *connection) ReadFrame() (frame.Frame, error) {
	return c.fs.ReadFrame()
}

func (c *connection) ClientType() ClientType {
	return c.clientType
}

func (c *connection) WriteFrame(f frame.Frame) error {
	return c.fs.WriteFrame(f)
}

func (c *connection) CloseWithError(errString string) error {
	return c.conn.CloseWithError(YomoCloseErrorCode, errString)
}

// YomoCloseErrorCode is the error code for close quic Connection for yomo.
// If the Connection implemented by quic is closed, the quic ApplicationErrorCode is always 0x13.
const YomoCloseErrorCode = quic.ApplicationErrorCode(0x13)
