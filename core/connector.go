package core

import (
	"fmt"
	"io"
	"math/rand"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

type app struct {
	id       string // app id
	name     string // app name
	observed []byte // data tags
}

func (a *app) ID() string {
	return a.id
}

func (a *app) Name() string {
	return a.name
}

var _ Connector = &connector{}

// Connector is a interface to manage the connections and applications.
type Connector interface {
	// Add a connection.
	Add(connID string, stream io.ReadWriteCloser)
	// Remove a connection.
	Remove(connID string)
	// Get a connection by connection id.
	Get(connID string) io.ReadWriteCloser
	// GetConnIDs gets the connection ids by appID, name and tag.
	GetConnIDs(appID string, name string, tags byte) []string
	// Write a DataFrame to a connection.
	Write(f *frame.DataFrame, toID string) error
	// GetSnapshot gets the snapshot of all connections.
	GetSnapshot() map[string]io.ReadWriteCloser

	// App gets the app by connID.
	App(connID string) (*app, bool)
	// AppID gets the ID of app by connID.
	AppID(connID string) (string, bool)
	// AppName gets the name of app by connID.
	AppName(connID string) (string, bool)
	// LinkApp links the app and connection.
	LinkApp(connID string, appID string, name string, observed []byte)
	// UnlinkApp removes the app by connID.
	UnlinkApp(connID string, appID string, name string)

	// Clean the connector.
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

// Add a connection.
func (c *connector) Add(connID string, stream io.ReadWriteCloser) {
	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
	c.conns.Store(connID, stream)
}

// Remove a connection.
func (c *connector) Remove(connID string) {
	logger.Debugf("%sconnector remove: connID=%s", ServerLogPrefix, connID)
	c.conns.Delete(connID)
	// c.funcs.Delete(connID)
	c.apps.Delete(connID)
}

// Get a connection by connection id.
func (c *connector) Get(connID string) io.ReadWriteCloser {
	logger.Debugf("%sconnector get connection: connID=%s", ServerLogPrefix, connID)
	if stream, ok := c.conns.Load(connID); ok {
		return stream.(io.ReadWriteCloser)
	}
	return nil
}

// App gets the app by connID.
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

// AppID gets the ID of app by connID.
func (c *connector) AppID(connID string) (string, bool) {
	if app, ok := c.App(connID); ok {
		return app.id, true
	}
	return "", false
}

// AppName gets the name of app by connID.
func (c *connector) AppName(connID string) (string, bool) {
	if app, ok := c.App(connID); ok {
		return app.name, true
	}
	return "", false
}

// GetConnIDs gets the connection ids by appID, name and tag.
func (c *connector) GetConnIDs(appID string, name string, tag byte) []string {
	connIDs := make([]string, 0)

	c.apps.Range(func(key interface{}, val interface{}) bool {
		app := val.(*app)
		if app.id == appID && app.name == name {
			for _, v := range app.observed {
				if v == tag {
					connIDs = append(connIDs, key.(string))
					break
				}
			}
		}
		return true
	})

	n := len(connIDs)
	if n > 1 {
		index := rand.Intn(n)
		return connIDs[index : index+1]
	} else {
		return connIDs
	}
}

// Write a DataFrame to a connection.
func (c *connector) Write(f *frame.DataFrame, toID string) error {
	targetStream := c.Get(toID)
	if targetStream == nil {
		logger.Warnf("%swill write to: [%s], target stream is nil", ServerLogPrefix, toID)
		return fmt.Errorf("target[%s] stream is nil", toID)
	}
	_, err := targetStream.Write(f.Encode())
	return err
}

// GetSnapshot gets the snapshot of all connections.
func (c *connector) GetSnapshot() map[string]io.ReadWriteCloser {
	result := make(map[string]io.ReadWriteCloser)
	c.conns.Range(func(key interface{}, val interface{}) bool {
		result[key.(string)] = val.(io.ReadWriteCloser)
		return true
	})
	return result
}

// LinkApp links the app and connection.
func (c *connector) LinkApp(connID string, appID string, name string, observed []byte) {
	logger.Debugf("%sconnector link application: connID[%s] --> app[%s::%s]", ServerLogPrefix, connID, appID, name)
	c.apps.Store(connID, &app{appID, name, observed})
}

// UnlinkApp removes the app by connID.
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

// Clean the connector.
func (c *connector) Clean() {
	c.conns = sync.Map{}
	c.apps = sync.Map{}
}
