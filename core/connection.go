package core

import (
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	ynet "github.com/yomorun/yomo/core/net"
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
	fconn           ynet.FrameConn
	Logger          *slog.Logger
}

func newConnection(
	name string, id string, clientType ClientType, md metadata.M, tags []uint32,
	fconn ynet.FrameConn, logger *slog.Logger,
) *Connection {

	logger = logger.With("conn_id", id, "conn_name", name)
	if fconn != nil {
		logger.Info("new client connected", "remote_addr", fconn.RemoteAddr().String(), "client_type", clientType.String())
	}

	return &Connection{
		name:            name,
		id:              id,
		clientType:      clientType,
		metadata:        md,
		observeDataTags: tags,
		fconn:           fconn,
		Logger:          logger,
	}
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

func (c *Connection) ClientType() ClientType {
	return c.clientType
}

func (c *Connection) FrameConn() ynet.FrameConn {
	return c.fconn
}
