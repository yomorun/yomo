package core

import (
	"sync"

	"github.com/yomorun/yomo/pkg/logger"
)

var _ Connector = &connector{}

// Connector is a interface to manage the connections and applications.
type Connector interface {
	// Add a connection.
	Add(connID string, conn Connection)
	// Remove a connection.
	Remove(connID string)
	// Get a connection by connection id.
	Get(connID string) Connection
	// GetSnapshot gets the snapshot of all connections.
	GetSnapshot() map[string]string
	// GetSourceConns gets the connections by source observe tags.
	GetSourceConns(sourceID string, tags byte) []Connection
	// LinkSource links the source and connection.
	// LinkSource(connID string, id string, name string, sourceID string, observed []byte)
	// Clean the connector.
	Clean()
}

type connector struct {
	conns sync.Map
}

func newConnector() Connector {
	return &connector{conns: sync.Map{}}
}

// Add a connection.
func (c *connector) Add(connID string, conn Connection) {
	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
	c.conns.Store(connID, conn)
}

// Remove a connection.
func (c *connector) Remove(connID string) {
	logger.Debugf("%sconnector remove: connID=%s", ServerLogPrefix, connID)
	c.conns.Delete(connID)
}

// Get a connection by connection id.
func (c *connector) Get(connID string) Connection {
	logger.Debugf("%sconnector get connection: connID=%s", ServerLogPrefix, connID)
	if conn, ok := c.conns.Load(connID); ok {
		return conn.(Connection)
	}
	return nil
}

// GetSourceConns gets the source connection by tag.
func (c *connector) GetSourceConns(sourceID string, tag byte) []Connection {
	// connIDs := make([]string, 0)

	// c.sources.Range(func(key interface{}, val interface{}) bool {
	// 	app := val.(*app)
	// 	for _, v := range app.observed {
	// 		if v == tag {
	// 			connIDs = append(connIDs, key.(string))
	// 			// break
	// 		}
	// 	}
	// 	return true
	// })

	// return connection list
	conns := make([]Connection, 0)

	c.conns.Range(func(key interface{}, val interface{}) bool {
		conn := val.(Connection)
		for _, v := range conn.ObserveDataTags() {
			if v == tag {
				conns = append(conns, conn)
			}
		}
		return true
	})

	return conns
}

// Write a Frame to a connection.
// func (c *connector) Write(f frame.Frame, toID string) error {
// 	targetStream := c.Get(toID)
// 	if targetStream == nil {
// 		logger.Warnf("%swill write to: [%s], target stream is nil", ServerLogPrefix, toID)
// 		return fmt.Errorf("target[%s] stream is nil", toID)
// 	}
// 	c.mu.Lock()
// 	_, err := targetStream.Write(f.Encode())
// 	c.mu.Unlock()
// 	return err
// }

// GetSnapshot gets the snapshot of all connections.
func (c *connector) GetSnapshot() map[string]string {
	result := make(map[string]string)
	c.conns.Range(func(key interface{}, val interface{}) bool {
		connID := key.(string)
		conn := val.(Connection)
		result[connID] = conn.Name()
		return true
	})
	return result
}

// LinkSource links the source and connection.
// func (c *connector) LinkSource(connID string, id string, name string, sourceID string, observed []byte) {
// 	logger.Debugf("%sconnector link source: connID[%s] --> source[%s]", ServerLogPrefix, connID, name)
// 	c.sources.Store(connID, &app{id, name, observed, sourceID})
// }

// Clean the connector.
func (c *connector) Clean() {
	c.conns = sync.Map{}
}
