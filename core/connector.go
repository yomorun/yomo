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
	// GetSourceConnIDs gets the connection ids by source observe tag.
	GetSourceConnIDs(tags byte) []string
	// LinkSource links the source and connection.
	LinkSource(connID string, name string, observed []byte)
	// Clean the connector.
	Clean()
}

type connector struct {
	conns sync.Map
	sources sync.Map
}

func newConnector() Connector {
	return &connector{
	conns   sync.Map
	sources sync.Map
	}
}

// Add a connection.
func (c *connector) Add(connID string, conn Connection) {
	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
	c.conns.Store(connID, conn)
}

// func (c *connector) AddSource(connID string, stream io.ReadWriteCloser) {
// 	logger.Debugf("%sconnector add: connID=%s", ServerLogPrefix, connID)
// 	c.sconns.Store(connID, stream)
// }

// Remove a connection.
func (c *connector) Remove(connID string) {
	logger.Debugf("%sconnector remove: connID=%s", ServerLogPrefix, connID)
	c.conns.Delete(connID)
	c.sources.Delete(connID)
}

// Get a connection by connection id.
func (c *connector) Get(connID string) Connection {
	logger.Debugf("%sconnector get connection: connID=%s", ServerLogPrefix, connID)
	if conn, ok := c.conns.Load(connID); ok {
		return conn.(Connection)
	}
	return nil
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

// Write a Frame to a connection.
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
func (c *connector) LinkSource(connID string, name string, observed []byte) {
	logger.Debugf("%sconnector link source: connID[%s] --> source[%s]", ServerLogPrefix, connID, name)
	c.sources.Store(connID, &app{name, observed})
}

// Clean the connector.
func (c *connector) Clean() {
	c.conns = sync.Map{}
}
