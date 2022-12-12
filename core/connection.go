package core

import (
	"io"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
)

// Connection wraps the specific io connections (typically quic.Connection) to transfer y3 frames
type Connection interface {
	io.Closer

	// Name returns the name of the connection, which is set by clients
	Name() string
	// ClientID connection client ID
	ClientID() string
	// ClientType returns the type of the client (Source | SFN | UpstreamZipper)
	ClientType() ClientType
	// Metadata returns the extra info of the application
	Metadata() metadata.Metadata
	// Write should goroutine-safely send y3 frames to peer side
	Write(f frame.Frame) error
	// ObserveDataTags observed data tags
	ObserveDataTags() []frame.Tag
}

type connection struct {
	name       string
	clientType ClientType
	metadata   metadata.Metadata
	stream     io.ReadWriteCloser
	clientID   string
	observed   []frame.Tag // observed data tags
	mu         sync.Mutex
	closed     bool
	logger     *slog.Logger
}

func newConnection(
	name string,
	clientID string,
	clientType ClientType,
	metadata metadata.Metadata,
	stream io.ReadWriteCloser,
	observed []frame.Tag,
	logger *slog.Logger,
) Connection {
	logger.Debug("new connecton")
	return &connection{
		name:       name,
		clientID:   clientID,
		clientType: clientType,
		observed:   observed,
		metadata:   metadata,
		stream:     stream,
		closed:     false,
	}
}

// Close implements io.Close interface
func (c *connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if !c.closed {
		c.closed = true
		err = c.stream.Close()
	}
	return err
}

// Name returns the name of the connection, which is set by clients
func (c *connection) Name() string {
	return c.name
}

// ClientType returns the type of the connection (Source | SFN | UpstreamZipper)
func (c *connection) ClientType() ClientType {
	return c.clientType
}

// Metadata returns the extra info of the application
func (c *connection) Metadata() metadata.Metadata {
	return c.metadata
}

// Write should goroutine-safely send y3 frames to peer side
func (c *connection) Write(f frame.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		c.logger.Warn("client stream is closed")
		return nil
	}
	_, err := c.stream.Write(f.Encode())
	return err
}

// ObserveDataTags observed data tags
func (c *connection) ObserveDataTags() []frame.Tag {
	return c.observed
}

// ClientID connection client ID
func (c *connection) ClientID() string {
	return c.clientID
}
