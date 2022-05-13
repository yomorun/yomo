package core

import (
	"io"
	"sync"

	"github.com/yomorun/yomo/core/frame"
)

// Connection wraps the specific io connections (typically quic.Connection) to transfer y3 frames
type Connection interface {
	io.Closer

	// Name returns the name of the connection, which is set by clients
	Name() string
	// ClientType returns the type of the client (Source | SFN | UpstreamZipper)
	ClientType() ClientType
	// AppInfo returns the extra application info
	AppInfo() AppInfo
	// Write should goroutine-safely send y3 frames to peer side
	Write(f frame.Frame) error
}

type connection struct {
	name       string
	clientType ClientType
	appInfo    AppInfo
	stream     io.ReadWriteCloser
	mu         sync.Mutex
}

func newConnection(name string, clientType ClientType, appInfo AppInfo, stream io.ReadWriteCloser) Connection {
	return &connection{
		name:       name,
		clientType: clientType,
		appInfo:    appInfo,
		stream:     stream,
	}
}

// Close implements io.Close interface
func (c *connection) Close() error {
	return c.stream.Close()
}

// Name returns the name of the connection, which is set by clients
func (c *connection) Name() string {
	return c.name
}

// ClientType returns the type of the connection (Source | SFN | UpstreamZipper)
func (c *connection) ClientType() ClientType {
	return c.clientType
}

// AppInfo returns the extra application info
func (c *connection) AppInfo() AppInfo {
	return c.appInfo
}

// Write should goroutine-safely send y3 frames to peer side
func (c *connection) Write(f frame.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.stream.Write(f.Encode())
	return err
}
