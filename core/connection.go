package core

import (
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/listener"
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
	ConnectionInfo
	FrameConn() listener.FrameConn
}

type connection struct {
	name            string
	id              string
	clientType      ClientType
	metadata        metadata.M
	observeDataTags []uint32
	fconn           listener.FrameConn
}

func newConnection(
	name string, id string, clientType ClientType, md metadata.M, tags []uint32,
	fconn listener.FrameConn) *connection {
	return &connection{
		name:            name,
		id:              id,
		clientType:      clientType,
		metadata:        md,
		observeDataTags: tags,
		fconn:           fconn,
	}
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

func (c *connection) ClientType() ClientType {
	return c.clientType
}

func (c *connection) FrameConn() listener.FrameConn {
	return c.fconn
}
