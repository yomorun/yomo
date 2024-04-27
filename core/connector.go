package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrConnectorClosed will be returned if the Connector has been closed.
var ErrConnectorClosed = errors.New("yomo: connector closed")

type connector struct {
	// ctx and ctxCancel manage the lifescyle of Connector.
	ctx       context.Context
	ctxCancel context.CancelFunc

	// connections stores data connections.
	connections sync.Map
}

// Connector manages connections and provides a centralized way for getting and setting streams.
type Connector interface {
	// Get retrieves the Connection with the specified id.
	// If the Connector does not have a connection with the given id, return nil and false.
	// If a Connector is closed, the function returns ErrConnectorClosed.
	Get(id uint64) (*Connection, bool, error)
	// Store stores Connection to Connector,
	// The newer connection will replaces the older one.
	// If a Connector is closed, the function returns ErrConnectorClosed.
	Store(connID uint64, conn *Connection) error
	// Remove removes the connection with the specified connID.
	// If the Connector does not have a connection with the given connID, no action is taken.
	// If a Connector is closed, the function returns ErrConnectorClosed.
	Remove(connID uint64) error
	// Close closes all connections in the Connector and resets the Connector to a closed state.
	// After closing, the Connector cannot be used anymore.
	// Calling close multiple times has no effect.
	Close() error
	// Snapshot returns a map that contains a snapshot of all connections.
	// The resulting map uses the connID as the key and the connection name as the value.
	// This function is typically used to monitor the status of the Connector.
	Snapshot() map[string]string
}

// NewConnector returns an initial Connector.
func NewConnector(ctx context.Context) Connector {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &connector{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}
}

func (c *connector) Store(connID uint64, conn *Connection) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.connections.Store(connID, conn)

	return nil
}

func (c *connector) Remove(connID uint64) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.connections.Delete(connID)

	return nil
}

func (c *connector) Get(id uint64) (*Connection, bool, error) {
	select {
	case <-c.ctx.Done():
		return nil, false, ErrConnectorClosed
	default:
	}

	v, ok := c.connections.Load(id)
	if !ok {
		return nil, false, nil
	}

	return v.(*Connection), true, nil
}

func (c *connector) Snapshot() map[string]string {
	result := make(map[string]string)

	c.connections.Range(func(key interface{}, val interface{}) bool {
		var (
			id   = key.(uint64)
			conn = val.(*Connection)
		)

		result[fmt.Sprintf("%d", id)] = conn.Name()
		return true
	})

	return result
}

func (c *connector) Close() error {
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
