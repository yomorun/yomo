package core

import (
	"context"
	"errors"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

// ErrConnectorClosed will be returned if the connector has been closed.
var ErrConnectorClosed = errors.New("yomo: connector closed")

// The Connector class manages data streams and provides a centralized way to get and set streams.
type Connector struct {
	// ctx and ctxCancel manage the lifescyle of Connector.
	ctx       context.Context
	ctxCancel context.CancelFunc

	streams sync.Map
	logger  *slog.Logger
}

// NewConnector returns an initial Connector.
func NewConnector(ctx context.Context, logger *slog.Logger) *Connector {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &Connector{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		logger:    logger,
	}
}

// Add adds DataStream to Connector,
// If the streamID is the same twice, the new stream will replace the old stream.
func (c *Connector) Add(streamID string, stream DataStream) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.streams.Store(streamID, stream)

	c.logger.Debug("Connector add stream", "stream_id", streamID)
	return nil
}

// Remove removes the DataStream with the specified streamID.
// If the Connector does not have a stream with the given streamID, no action is taken.
func (c *Connector) Remove(streamID string) error {
	select {
	case <-c.ctx.Done():
		return ErrConnectorClosed
	default:
	}

	c.streams.Delete(streamID)
	c.logger.Debug("Connector remove stream", "stream_id", streamID)

	return nil
}

// Get retrieves the DataStream with the specified streamID.
// If the Connector does not have a stream with the given streamID, return nil and false.
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

// GetSourceStreams gets the streams with the specified source observe tag.
func (c *Connector) GetSourceStreams(sourceID string, tag frame.Tag) ([]DataStream, error) {
	select {
	case <-c.ctx.Done():
		return []DataStream{}, ErrConnectorClosed
	default:
	}

	streams := make([]DataStream, 0)

	c.streams.Range(func(key interface{}, val interface{}) bool {
		stream := val.(DataStream)

		for _, v := range stream.ObserveDataTags() {
			if v == tag &&
				stream.StreamType() == StreamTypeSource &&
				stream.ID() == sourceID {
				streams = append(streams, stream)
			}
		}
		return true
	})

	return streams, nil
}

// GetSnapshot returnsa snapshot of all streams.
// The resulting map uses streamID as the key and stream name as the value.
// This function is typically used to monitor the status of the Connector.
func (c *Connector) GetSnapshot() map[string]string {
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

// Close cleans all stream of Connector and reset Connector to closed status.
// The Connector can't be use after close.
func (c *Connector) Close() {
	c.ctxCancel()

	c.streams.Range(func(key, value any) bool {
		c.streams.Delete(key)
		return true
	})
}
