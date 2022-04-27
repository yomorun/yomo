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
	name     string // app name
	observed []byte // data tags
}

// func (a *app) ID() string {
// 	return a.id
// }

func (a *app) Name() string {
	return a.name
}

var _ Connector = &connector{}

// Connector is a interface to manage the connections and applications.
type Connector interface {
	// Add a connection. stream is quic.Stream
	Add(connID string, stream io.ReadWriteCloser)
	// Remove a connection.
	Remove(connID string)
	// Get a connection by connection id.
	Get(connID string) io.ReadWriteCloser
	// GetConnIDs gets the connection ids by name and tag.
	GetConnIDs(name string, tag byte) []string
	// GetSourceConnIDs gets the connection ids by source observe tag.
	GetSourceConnIDs(tag byte) []string
	// Write a Frame to a connection.
	Write(f frame.Frame, toID string) error
	// GetSnapshot gets the snapshot of all connections.
	GetSnapshot() map[string]io.ReadWriteCloser
	// App gets the app by connID.
	App(connID string) (*app, bool)
	// AppID gets the ID of app by connID.
	// AppID(connID string) (string, bool)
	// AppName gets the name of app by connID.
	AppName(connID string) (string, bool)
	// LinkApp links the app and connection.
	LinkApp(connID string, name string, observed []byte)
	// LinkSource links the source and connection.
	LinkSource(connID string, name string, observed []byte)
	// UnlinkApp removes the app by connID.
	UnlinkApp(connID string, name string)
	// ExistsApp check app exists
	ExistsApp(name string) bool

	// Clean the connector.
	Clean()
}

type connector struct {
	conns   sync.Map
	apps    sync.Map
	sources sync.Map
	mu      sync.Mutex
}

func newConnector() Connector {
	return &connector{
		conns:   sync.Map{},
		apps:    sync.Map{},
		sources: sync.Map{},
		mu:      sync.Mutex{},
	}
}

// Add a connection.
func (c *connector) Add(connID string, stream io.ReadWriteCloser) {
	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
	c.conns.Store(connID, stream)
}

// func (c *connector) AddSource(connID string, stream io.ReadWriteCloser) {
// 	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
// 	c.sconns.Store(connID, stream)
// }

// Remove a connection.
func (c *connector) Remove(connID string) {
	logger.Debugf("%sconnector remove: connID=%s", ServerLogPrefix, connID)
	c.conns.Delete(connID)
	// c.funcs.Delete(connID)
	c.apps.Delete(connID)
	c.sources.Delete(connID)
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
			logger.Debugf("%sconnector get app=%s, connID=%s", ServerLogPrefix, app.name, connID)
			return app, true
		}
		logger.Warnf("%sconnector get app convert fails, connID=%s", ServerLogPrefix, connID)
		return nil, false
	}
	logger.Warnf("%sconnector get app is nil, connID=%s", ServerLogPrefix, connID)
	return nil, false
}

// AppName gets the name of app by connID.
func (c *connector) AppName(connID string) (string, bool) {
	if app, ok := c.App(connID); ok {
		return app.name, true
	}
	return "", false
}

// GetConnIDs gets the connection ids by name and tag.
func (c *connector) GetConnIDs(name string, tag byte) []string {
	connIDs := make([]string, 0)

	c.apps.Range(func(key interface{}, val interface{}) bool {
		app := val.(*app)
		if app.name == name {
			for _, v := range app.observed {
				if v == tag {
					connIDs = append(connIDs, key.(string))
					break
				}
			}
		}
		return true
	})

	if n := len(connIDs); n > 1 {
		index := rand.Intn(n)
		return connIDs[index : index+1]
	}

	return connIDs
}

// GetSourceConnIDs gets the source connection ids by tag.
func (c *connector) GetSourceConnIDs(tag byte) []string {
	connIDs := make([]string, 0)

	c.sources.Range(func(key interface{}, val interface{}) bool {
		app := val.(*app)
		for _, v := range app.observed {
			if v == tag {
				connIDs = append(connIDs, key.(string))
				// break
			}
		}
		return true
	})

	return connIDs
}

// Write a DataFrame to a connection.
func (c *connector) Write(f frame.Frame, toID string) error {
	targetStream := c.Get(toID)
	if targetStream == nil {
		logger.Warnf("%swill write to: [%s], target stream is nil", ServerLogPrefix, toID)
		return fmt.Errorf("target[%s] stream is nil", toID)
	}
	c.mu.Lock()
	_, err := targetStream.Write(f.Encode())
	c.mu.Unlock()
	return err
}

// WriteWithCallback a DataFrame to a connection.
func (c *connector) WriteWithCallback(f frame.Frame, toID string, callback func(stream io.ReadWriteCloser)) error {
	targetStream := c.Get(toID)
	if targetStream == nil {
		logger.Warnf("%swill write to: [%s], target stream is nil", ServerLogPrefix, toID)
		return fmt.Errorf("target[%s] stream is nil", toID)
	}
	c.mu.Lock()
	_, err := targetStream.Write(f.Encode())
	c.mu.Unlock()
	if err != nil {
		return err
	}
	callback(targetStream)
	return nil
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
func (c *connector) LinkApp(connID string, name string, observed []byte) {
	logger.Debugf("%sconnector link application: connID[%s] --> app[%s]", ServerLogPrefix, connID, name)
	c.apps.Store(connID, &app{name, observed})
}

// LinkSource links the source and connection.
func (c *connector) LinkSource(connID string, name string, observed []byte) {
	logger.Debugf("%sconnector link source: connID[%s] --> source[%s]", ServerLogPrefix, connID, name)
	c.sources.Store(connID, &app{name, observed})
}

// UnlinkApp removes the app by connID.
func (c *connector) UnlinkApp(connID string, name string) {
	logger.Debugf("%sconnector unlink application: connID[%s] x-> app[%s]", ServerLogPrefix, connID, name)
	c.apps.Delete(connID)
}

// ExistsApp check app exists
func (c *connector) ExistsApp(name string) bool {
	var found bool
	c.apps.Range(func(key interface{}, val interface{}) bool {
		app := val.(*app)
		if app.name == name {
			found = true
			return false
		}
		return true
	})

	return found
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
