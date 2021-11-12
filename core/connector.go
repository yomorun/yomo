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

type app struct {
	id   string // app id
	name string // app name
}

func newApp(id string, name string) *app {
	return &app{id: id, name: name}
}

func (a *app) ID() string {
	return a.id
}

func (a *app) Name() string {
	return a.name
}

var _ Connector = &connector{}

type Connector interface {
	Add(connID string, stream *quic.Stream)
	Remove(connID string)
	Get(connID string) *quic.Stream
	ConnID(appID string, name string) (string, bool)
	Write(f *frame.DataFrame, fromID string, toID string) error
	GetSnapshot() map[string]*quic.Stream

	App(connID string) (*app, bool)
	AppID(connID string) (string, bool)
	AppName(connID string) (string, bool)
	LinkApp(connID string, appID string, name string)
	UnlinkApp(connID string, appID string, name string)

	Clean()
}

type connector struct {
	conns sync.Map
	apps  sync.Map
}

func newConnector() Connector {
	return &connector{
		conns: sync.Map{},
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
	// c.funcs.Delete(connID)
	c.apps.Delete(connID)
}

func (c *connector) Get(connID string) *quic.Stream {
	logger.Debugf("%sconnector get connection: connID=%s", ServerLogPrefix, connID)
	if stream, ok := c.conns.Load(connID); ok {
		return stream.(*quic.Stream)
	}
	return nil
}

func (c *connector) App(connID string) (*app, bool) {
	if result, found := c.apps.Load(connID); found {
		app, ok := result.(*app)
		if ok {
			logger.Debugf("%sconnector get app=%s::%s, connID=%s", ServerLogPrefix, app.id, app.name, connID)
			return app, true
		}
		logger.Warnf("%sconnector get app convert fails, connID=%s", ServerLogPrefix, connID)
		return nil, false
	}
	logger.Warnf("%sconnector get app is nil, connID=%s", ServerLogPrefix, connID)
	return nil, false
}

func (c *connector) AppID(connID string) (string, bool) {
	if app, ok := c.App(connID); ok {
		return app.id, true
	}
	return "", false
}

func (c *connector) AppName(connID string) (string, bool) {
	if app, ok := c.App(connID); ok {
		return app.name, true
	}
	return "", false
}

func (c *connector) ConnID(appID string, name string) (string, bool) {
	var connID string
	var ok bool

	c.apps.Range(func(key interface{}, val interface{}) bool {
		app := val.(*app)
		if app.id == appID && app.name == name {
			connID = key.(string)
			ok = true
			return false
		}
		return true
	})
	if !ok {
		logger.Warnf("%snot available connection, name=%s::%s", ServerLogPrefix, appID, name)
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

func (c *connector) LinkApp(connID string, appID string, name string) {
	logger.Debugf("%sconnector link application: connID[%s] --> app[%s::%s]", ServerLogPrefix, connID, appID, name)
	c.apps.Store(connID, newApp(appID, name))
}

func (c *connector) UnlinkApp(connID string, appID string, name string) {
	logger.Debugf("%sconnector unlink application: connID[%s] x-> app[%s::%s]", ServerLogPrefix, connID, appID, name)
	c.apps.Delete(connID)
}

// func (c *connector) RemoveApp(appID string) {
// 	logger.Debugf("%sconnector unlink application: connID[%s] x-> app[%s]", ServerLogPrefix, connID, appID)
// 	c.apps.Range(func(key interface{},val interface{})bool{
// 		return true
// 	})
// 	c.rapps.Delete(appID)
// }

// func (c *connector) AppConns(appID string) []string {
// 	conns := make([]string, 0)
// 	c.apps.Range(func(key interface{},val interface{})bool{
// 		if val.(string)==appID{
// 			conns=append(conns,key.(string))
// 		}
// 	})
// 	return conns
// }

func (c *connector) Clean() {
	c.conns = sync.Map{}
	c.apps = sync.Map{}
}
