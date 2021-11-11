package core

import (
	"fmt"
	"sync"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

type connStream struct {
	id     string       // connection id (remote addr)
	stream *quic.Stream // quic stream
}

var _ Connector = &connector{}

type Connector interface {
	Add(connID string, stream *quic.Stream)
	Remove(connID string)
	Get(connID string) *quic.Stream
	Name(connID string) (string, bool)
	ConnID(name string) (string, bool)
	Write(f *frame.DataFrame, fromID string, toID string) error
	GetSnapshot() map[string]*quic.Stream
	Link(connID string, name string)
	Unlink(connID string, name string)
	LinkApp(connID string, appID string)
	UnlinkApp(connID string, appID string)
	AppID(connID string) (string, bool)
	Clean()
}

type connector struct {
	conns sync.Map
	links sync.Map
	apps  sync.Map
}

func newConnector() Connector {
	return &connector{
		conns: sync.Map{},
		links: sync.Map{},
		apps:  sync.Map{},
	}
}

func (c *connector) Add(connID string, stream *quic.Stream) {
	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
	c.conns.Store(connID, stream)
}

func (c *connector) Remove(connID string) {
	logger.Debugf("%sconnector remove: connID=%s", ServerLogPrefix, connID)
	c.conns.Delete(connID)
	c.links.Delete(connID)
	c.apps.Delete(connID)
}

func (c *connector) Get(connID string) *quic.Stream {
	logger.Debugf("%sconnector get connection: connID=%s", ServerLogPrefix, connID)
	if stream, ok := c.conns.Load(connID); ok {
		return stream.(*quic.Stream)
	}
	return nil
}

func (c *connector) Name(connID string) (string, bool) {
	if name, ok := c.links.Load(connID); ok {
		logger.Debugf("%sconnector get name=%s, connID=%s", ServerLogPrefix, name.(string), connID)
		return name.(string), true
	}
	logger.Warnf("%sconnector get name is empty, connID=%s", ServerLogPrefix, connID)
	return "", false
}

func (c *connector) ConnID(name string) (string, bool) {
	var connID string
	var ok bool
	c.links.Range(func(key interface{}, val interface{}) bool {
		if val.(string) == name {
			connID = key.(string)
			ok = true
			return false
		}
		return true
	})
	if !ok {
		logger.Warnf("%snot available connection, name=%s", ServerLogPrefix, name)
		return "", false
	}
	logger.Debugf("%suse connection: connID=%s", ServerLogPrefix, connID)
	return connID, true
}

func (c *connector) Write(f *frame.DataFrame, fromID string, toID string) error {
	targetStream := c.Get(toID)
	if targetStream == nil {
		logger.Warnf("%swill write to: [%s] -> [%s], target stream is nil", ServerLogPrefix, fromID, toID)
		return fmt.Errorf("target[%s] stream is nil", toID)
	}
	_, err := (*targetStream).Write(f.Encode())
	return err
}

func (c *connector) GetSnapshot() map[string]*quic.Stream {
	result := make(map[string]*quic.Stream)
	c.conns.Range(func(key interface{}, val interface{}) bool {
		result[key.(string)] = val.(*quic.Stream)
		return true
	})
	return result
}

func (c *connector) Link(connID string, name string) {
	logger.Debugf("%sconnector link: connID[%s] --> SFN[%s]", ServerLogPrefix, connID, name)
	c.links.Store(connID, name)
}

func (c *connector) Unlink(connID string, name string) {
	logger.Debugf("%sconnector unlink: connID[%s] x-> SFN[%s]", ServerLogPrefix, connID, name)
	c.links.Delete(connID)
}

func (c *connector) LinkApp(connID string, appID string) {
	logger.Debugf("%sconnector link application: connID[%s] --> app[%s]", ServerLogPrefix, connID, appID)
	c.apps.Store(connID, appID)
}

func (c *connector) UnlinkApp(connID string, appID string) {
	logger.Debugf("%sconnector unlink application: connID[%s] x-> app[%s]", ServerLogPrefix, connID, appID)
	c.apps.Delete(connID)
}

func (c *connector) AppID(connID string) (string, bool) {
	if appID, ok := c.apps.Load(connID); ok {
		logger.Debugf("%sconnector get appID=%s, connID=%s", ServerLogPrefix, appID.(string), connID)
		return appID.(string), true
	}
	logger.Warnf("%sconnector get appID is empty, connID=%s", ServerLogPrefix, connID)
	return "", false
}

func (c *connector) Clean() {
	c.conns = sync.Map{}
	c.links = sync.Map{}
	c.apps = sync.Map{}
}
