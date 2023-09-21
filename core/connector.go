package core

import (
	"context"
	"errors"
	"sync"
)

// ErrConnectorClosed will be returned if the Connector has been closed.
var ErrConnectorClosed = errors.New("yomo: connector closed")

// Connector manages connections and provides a centralized way for getting and setting streams.
type Connector struct {
	// ctx and ctxCancel manage the lifescyle of Connector.
	ctx       context.Context
	ctxCancel context.CancelFunc

	// connections stores data connections.
	connections sync.Map
}

// NewConnector returns an initial Connector.
func NewConnector(ctx context.Context) *Connector {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &Connector{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}
}

// Store stores Connection to Connector,
// If the connID is the same twice, the new conntion will replace the old conntion.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Store(connID string, conn Connection) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.connections.Store(connID, conn)

	return nil
}

// Remove removes the conntion with the specified connID.
// If the Connector does not have a conntion with the given connID, no action is taken.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Remove(connID string) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.connections.Delete(connID)

	return nil
}

// Get retrieves the Connection with the specified connID.
// If the Connector does not have a conntion with the given connID, return nil and false.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Get(connID string) (Connection, bool, error) {
	select {
	case <-c.ctx.Done():
		return nil, false, ErrConnectorClosed
	default:
	}

	v, ok := c.connections.Load(connID)
	if !ok {
		return nil, false, nil
	}

	connection := v.(Connection)

	return connection, true, nil
}

// FindConnectionFunc is used to search for a specific connection within the Connector.
type FindConnectionFunc func(ConnectionInfo) bool

// Find searches a stream collection using the specified find function.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Find(findFunc FindConnectionFunc) ([]Connection, error) {
	select {
	case <-c.ctx.Done():
		return []Connection{}, ErrConnectorClosed
	default:
	}

	connections := make([]Connection, 0)
	c.connections.Range(func(key interface{}, val interface{}) bool {
		stream := val.(Connection)

		if findFunc(stream) {
			connections = append(connections, stream)
		}
		return true
	})

	return connections, nil
}

// Snapshot returns a map that contains a snapshot of all connections.
// The resulting map uses the connID as the key and the connection name as the value.
// This function is typically used to monitor the status of the Connector.
func (c *Connector) Snapshot() map[string]string {
	result := make(map[string]string)

	c.connections.Range(func(key interface{}, val interface{}) bool {
		var (
			streamID = key.(string)
			stream   = val.(Connection)
		)
		result[streamID] = stream.Name()
		return true
	})

	return result
}

// Close closes all connections in the Connector and resets the Connector to a closed state.
// After closing, the Connector cannot be used anymore.
// Calling close multiple times has no effect.
func (c *Connector) Close() error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.ctxCancel()

	c.connections.Range(func(key, value any) bool {
		c.connections.Delete(key)
		return true
	})

	return nil
}
