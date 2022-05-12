package core

import (
	"io"
	"sync"

	"github.com/yomorun/yomo/core/frame"
)

type Connection interface {
	io.Closer
	ConnID() string
	Name() string
	GetClientType() ClientType
	GetAppInfo() AppInfo
	Write(f frame.Frame) error
}

type connection struct {
	connID     string
	name       string
	clientType ClientType
	appInfo    AppInfo
	stream     io.ReadWriteCloser
	mu         sync.Mutex
}

func NewConnection(connID string, name string, clientType ClientType, appInfo AppInfo, stream io.ReadWriteCloser) Connection {
	return &connection{
		connID:     connID,
		name:       name,
		clientType: clientType,
		appInfo:    appInfo,
		stream:     stream,
	}
}

func (c *connection) Write(f frame.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.stream.Write(f.Encode())
	return err
}

func (c *connection) Close() error {
	return c.stream.Close()
}

func (c *connection) ConnID() string {
	return c.connID
}

func (c *connection) Name() string {
	return c.name
}

func (c *connection) GetClientType() ClientType {
	return c.clientType
}

func (c *connection) GetAppInfo() AppInfo {
	return c.appInfo
}
