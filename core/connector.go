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
	GetSnapshot() map[string]interface{}
	// ClearStats clears stats of all connections.
	ClearStats()
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

// GetSnapshot gets the snapshot of all connections.
func (c *connector) GetSnapshot() map[string]interface{} {
	result := make(map[string]interface{})
	c.conns.Range(func(key interface{}, val interface{}) bool {
		connID := key.(string)
		conn := val.(Connection)
		result[connID] = conn.GetSnapshot()
		return true
	})
	return result
}

// ClearStats clears stats of all connections.
func (c *connector) ClearStats() {
	c.conns.Range(func(_ interface{}, val interface{}) bool {
		conn := val.(Connection)
		conn.ClearStats()
		return true
	})
}

// Clean the connector.
func (c *connector) Clean() {
	c.conns = sync.Map{}
}
