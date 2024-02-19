package core

import (
	"sync/atomic"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
)

var increment uint64

// incrID generates next increment ID.
func incrID() uint64 {
	return atomic.AddUint64(&increment, 1)
}

// ConnectionInfo holds the information of connection.
type ConnectionInfo interface {
	// ID is the ID generated by server.
	ID() uint64
	// ClientID represents a client ID, the ClientID generated by client.
	ClientID() string
	// Name returns the name of the connection, which is set by clients.
	Name() string
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
	id              uint64
	name            string
	clientID        string
	clientType      ClientType
	metadata        metadata.M
	observeDataTags []uint32
	fconn           frame.Conn
	Logger          *slog.Logger
}

func newConnection(
	id uint64,
	name string, clientID string, clientType ClientType, md metadata.M, tags []uint32,
	fconn frame.Conn, logger *slog.Logger,
) *Connection {

	logger = logger.With("conn_id", clientID, "conn_name", name)

	return &Connection{
		id:              id,
		name:            name,
		clientID:        clientID,
		clientType:      clientType,
		metadata:        md,
		observeDataTags: tags,
		fconn:           fconn,
		Logger:          logger,
	}
}

// ID returns the increment ID.
func (c *Connection) ID() uint64 {
	return c.id
}

// ClientID returns the ID of client.
func (c *Connection) ClientID() string {
	return c.clientID
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

func (c *Connection) ClientType() ClientType {
	return c.clientType
}

func (c *Connection) FrameConn() frame.Conn {
	return c.fconn
}
