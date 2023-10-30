package core

import (
	"context"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
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

// Connection wraps connection and stream for transmitting frames, it can be
// used for reading and writing frames, and is managed by the Connector.
type Connection struct {
	name            string
	id              string
	clientType      ClientType
	metadata        metadata.M
	observeDataTags []uint32
	conn            quic.Connection
	fs              *FrameStream
	Logger          *slog.Logger
}

func newConnection(
	name string, id string, clientType ClientType, md metadata.M, tags []uint32,
	conn quic.Connection, fs *FrameStream, logger *slog.Logger) *Connection {

	logger = logger.With("conn_id", id, "conn_name", name)
	if conn != nil {
		logger.Info("new client connected", "remote_addr", conn.RemoteAddr().String(), "client_type", clientType.String())
	}

	return &Connection{
		name:            name,
		id:              id,
		clientType:      clientType,
		metadata:        md,
		observeDataTags: tags,
		conn:            conn,
		fs:              fs,
		Logger:          logger,
	}
}

// Close closes the connection.
func (c *Connection) Close() error {
	return c.fs.Close()
}

// Context returns the context of the connection.
func (c *Connection) Context() context.Context {
	return c.fs.Context()
}

// ID returns the connection ID.
func (c *Connection) ID() string {
	return c.id
}

// Metadata returns the extra info of the application.
func (c *Connection) Metadata() metadata.M {
	return c.metadata
}

// Name returns the name of the connection
func (c *Connection) Name() string {
	return c.name
}

// ObserveDataTags returns the observed data tags.
func (c *Connection) ObserveDataTags() []uint32 {
	return c.observeDataTags
}

// ReadFrame reads a frame from the connection.
func (c *Connection) ReadFrame() (frame.Frame, error) {
	return c.fs.ReadFrame()
}

// ClientType returns the client type of the connection.
func (c *Connection) ClientType() ClientType {
	return c.clientType
}

// WriteFrame writes a frame to the connection.
func (c *Connection) WriteFrame(f frame.Frame) error {
	return c.fs.WriteFrame(f)
}

// CloseWithError closes the connection with error.
func (c *Connection) CloseWithError(errString string) error {
	return c.conn.CloseWithError(YomoCloseErrorCode, errString)
}

// YomoCloseErrorCode is the error code for close quic Connection for yomo.
// If the Connection implemented by quic is closed, the quic ApplicationErrorCode is always 0x13.
const YomoCloseErrorCode = quic.ApplicationErrorCode(0x13)
