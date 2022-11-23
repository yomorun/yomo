package core

import (
	"sync"

	"github.com/yomorun/yomo/core/frame"
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
	// GetSourceConns gets the connections by source observe tag.
	GetSourceConns(sourceID string, tag frame.Tag) []Connection
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
	if conn, ok := c.conns.Load(connID); ok {
		return conn.(Connection)
	}
	return nil
}

// GetSourceConns gets the source connection by tag.
func (c *connector) GetSourceConns(sourceID string, tag frame.Tag) []Connection {
	conns := make([]Connection, 0)

	c.conns.Range(func(key interface{}, val interface{}) bool {
		conn := val.(Connection)
		for _, v := range conn.ObserveDataTags() {
			if v == tag && conn.ClientType() == ClientTypeSource && conn.ClientID() == sourceID {
				conns = append(conns, conn)
			}
		}
		return true
	})

	return conns
}

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

// Clean the connector.
func (c *connector) Clean() {
	c.conns = sync.Map{}
}
