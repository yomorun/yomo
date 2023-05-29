package core

import (
	"context"
	"errors"
	"sync"
)

// ErrConnectorClosed will be returned if the Connector has been closed.
var ErrConnectorClosed = errors.New("yomo: connector closed")

// Connector manages data streams and provides a centralized way for getting and setting streams.
type Connector struct {
	// ctx and ctxCancel manage the lifescyle of Connector.
	ctx       context.Context
	ctxCancel context.CancelFunc

	// streams stores data streams.
	streams sync.Map
}

// NewConnector returns an initial Connector.
func NewConnector(ctx context.Context) *Connector {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &Connector{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}
}

// Store stores DataStream to Connector,
// If the streamID is the same twice, the new stream will replace the old stream.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Store(streamID string, stream DataStream) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.streams.Store(streamID, stream)

	return nil
}

// Delete deletes the DataStream with the specified streamID.
// If the Connector does not have a stream with the given streamID, no action is taken.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Delete(streamID string) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.streams.Delete(streamID)

	return nil
}

// Get retrieves the DataStream with the specified streamID.
// If the Connector does not have a stream with the given streamID, return nil and false.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Get(streamID string) (DataStream, bool, error) {
	select {
	case <-c.ctx.Done():
		return nil, false, ErrConnectorClosed
	default:
	}

	v, ok := c.streams.Load(streamID)
	if !ok {
		return nil, false, nil
	}

	stream := v.(DataStream)

	return stream, true, nil
}

// FindStreamFunc is used to search for a specific stream within the Connector.
type FindStreamFunc func(StreamInfo) bool

// Find searches a stream collection using the specified find function.
// If Connector be closed, The function will return ErrConnectorClosed.
func (c *Connector) Find(findFunc FindStreamFunc) ([]DataStream, error) {
	select {
	case <-c.ctx.Done():
		return []DataStream{}, ErrConnectorClosed
	default:
	}

	streams := make([]DataStream, 0)
	c.streams.Range(func(key interface{}, val interface{}) bool {
		stream := val.(DataStream)

		if findFunc(stream) {
			streams = append(streams, stream)
		}
		return true
	})

	return streams, nil
}

// Snapshot returns a map that contains a snapshot of all streams.
// The resulting map uses the streamID as the key and the stream name as the value.
// This function is typically used to monitor the status of the Connector.
func (c *Connector) Snapshot() map[string]string {
	result := make(map[string]string)

	c.streams.Range(func(key interface{}, val interface{}) bool {
		var (
			streamID = key.(string)
			stream   = val.(DataStream)
		)
		result[streamID] = stream.Name()
		return true
	})

	return result
}

// Close closes all streams in the Connector and resets the Connector to a closed state.
// After closing, the Connector cannot be used anymore.
// Calling close multiple times has no effect.
func (c *Connector) Close() error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.ctxCancel()

	c.streams.Range(func(key, value any) bool {
		c.streams.Delete(key)
		return true
	})

	return nil
}
