package core

import (
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

var _ Connector = &connector{}

// Connector manages DataStream.
// Connector supports getting and setting stream from one place.
type Connector interface {
	// Add adds DataStream to Connector,
	// If the id is the same twice, the new stream replaces the old stream.
	Add(streamID string, stream DataStream)

	// Remove removes DataStream in streamID.
	// If Connector don't have a stream holds streamID, The Connector do nothing.
	Remove(streamID string)

	// Get gets DataStream in streamID.
	// If can't get a stream by stream, There will return nil and false.
	Get(streamID string) (DataStream, bool)

	// GetSnapshot gets the snapshot of all stream.
	// The map key is streamID, value is stream name, This function usually be used to
	// sniff the status of Connector.
	GetSnapshot() map[string]string

	// GetSourceConns gets the stream by source observe tag.
	GetSourceConns(sourceID string, tag frame.Tag) []DataStream

	// Clean cleans all stream of Connector and reset Connector to initial status.
	// TODO: add atomic.Bool to manage whether Connector is closed and rename to Close().
	Clean()
}

type connector struct {
	streams sync.Map
	logger  *slog.Logger
}

// newConnector returns an initial Connector.
func newConnector(logger *slog.Logger) Connector { return &connector{logger: logger} }

func (c *connector) Add(streamID string, stream DataStream) {
	c.streams.Store(streamID, stream)

	c.logger.Debug("Connector add stream", "stream_id", streamID)
}

func (c *connector) Remove(streamID string) {
	c.streams.Delete(streamID)

	c.logger.Debug("Connector remove stream", "stream_id", streamID)
}

func (c *connector) Get(streamID string) (DataStream, bool) {
	v, ok := c.streams.Load(streamID)

	if !ok {
		return nil, false
	}
	stream, ok := v.(DataStream)
	if !ok {
		return nil, false
	}

	return stream, true
}

// GetSourceConns gets the source connection by tag.
func (c *connector) GetSourceConns(sourceID string, tag frame.Tag) []DataStream {
	streams := make([]DataStream, 0)

	c.streams.Range(func(key interface{}, val interface{}) bool {
		stream, ok := val.(DataStream)
		if ok {
			return true
		}
		for _, v := range stream.ObserveDataTags() {
			if v == tag &&
				stream.StreamType() == StreamTypeSource &&
				stream.ID() == sourceID {
				streams = append(streams, stream)
			}
		}
		return true
	})

	return streams
}

func (c *connector) GetSnapshot() map[string]string {
	result := make(map[string]string)

	c.streams.Range(func(key interface{}, val interface{}) bool {
		streamID, ok := key.(string)
		if !ok {
			return true
		}
		stream, ok := val.(DataStream)
		if !ok {
			return true
		}
		result[streamID] = stream.Name()
		return true
	})

	return result
}

func (c *connector) Clean() {
	c.logger = nil
	c.streams.Range(func(key, value any) bool {
		c.streams.Delete(key)
		return true
	})
}
