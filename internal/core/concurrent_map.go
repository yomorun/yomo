package core

import (
	"sync"
	"sync/atomic"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/pkg/logger"
)

type connStream struct {
	id     string
	stream *quic.Stream
}

// ConcurrentMap store all stream function connections.
type ConcurrentMap struct {
	l             sync.RWMutex
	sfnCollection map[string][]connStream
	next          uint32
}

// NewConcurrentMap create a ConcurrentMap instance.
func NewConcurrentMap() *ConcurrentMap {
	return &ConcurrentMap{
		sfnCollection: make(map[string][]connStream),
	}
}

// Set will add a quic stream as stream function connection.
func (cmap *ConcurrentMap) Set(key string, connID string, stream *quic.Stream) {
	cmap.l.Lock()
	defer cmap.l.Unlock()
	connStreams := cmap.sfnCollection[key]
	connStream := connStream{id: connID, stream: stream}
	connStreams = append(connStreams, connStream)
	cmap.sfnCollection[key] = connStreams
}

// Get returns a quic stream which represents stream function connection.
func (cmap *ConcurrentMap) Get(key string) *quic.Stream {
	cmap.l.RLock()
	defer cmap.l.RUnlock()
	if val, ok := cmap.sfnCollection[key]; ok {
		l := len(val)
		if len(val) == 0 {
			logger.Debugf("not available stream, key=%s", key)
			return nil
		}
		if len(val) == 1 {
			logger.Debugf("stream[1st]")
			return val[0].stream
		}
		// get next session by Round Robin when has more sessions in this stream-fn.
		n := atomic.AddUint32(&cmap.next, 1)
		i := int(n) % l
		logger.Debugf("stream[%d]@%d", i, n)
		return val[i].stream
	}

	return nil
}

// Remove will remove a stream function connection.
func (cmap *ConcurrentMap) Remove(key string, connIDs ...string) {
	cmap.l.Lock()
	defer cmap.l.Unlock()
	if len(connIDs) == 0 {
		delete(cmap.sfnCollection, key)
		return
	}

	if connStreams, ok := cmap.sfnCollection[key]; ok {
		for i, connStream := range connStreams {
			for _, connID := range connIDs {
				if connStream.id == connID {
					connStreams = append(connStreams[:i], connStreams[i+1:]...)
				}
			}
		}
		cmap.sfnCollection[key] = connStreams
	}
}

// WriteToAll will dispatch data to all stream functions.
func (cmap *ConcurrentMap) WriteToAll(val []byte) {
	for _, targets := range cmap.sfnCollection {
		for _, target := range targets {
			(*target.stream).Write(val)
		}
	}
}

// GetCurrentSnapshot returns current snapshot of stream function connections.
func (cmap *ConcurrentMap) GetCurrentSnapshot() map[string][]*quic.Stream {
	result := make(map[string][]*quic.Stream)
	for key, connStreams := range cmap.sfnCollection {
		streams := make([]*quic.Stream, 0)
		for _, connStream := range connStreams {
			streams = append(streams, connStream.stream)
		}
		result[key] = streams
	}
	return result

}
