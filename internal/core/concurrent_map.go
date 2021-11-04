package core

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

type connStream struct {
	id     string       // connection rem_addr
	stream *quic.Stream // quic stream
}

// ConcurrentMap store all stream function connections.
type ConcurrentMap struct {
	l sync.RWMutex
	// stream function connection stream
	sfnCollection map[string][]connStream
	// key: connection ID, value: stream function name.
	connSfnMap map[string]string
	// user config stream functions
	funcBuckets map[int]string
	next        uint32
}

// NewConcurrentMap create a ConcurrentMap instance.
func NewConcurrentMap() *ConcurrentMap {
	return &ConcurrentMap{
		sfnCollection: make(map[string][]connStream),
		connSfnMap:    make(map[string]string),
		funcBuckets:   make(map[int]string),
	}
}

// Set will add stream function connection to collection.
func (cmap *ConcurrentMap) Set(token string, connID string, stream *quic.Stream) {
	cmap.l.Lock()
	defer cmap.l.Unlock()
	connStreams := cmap.sfnCollection[token]
	connStream := connStream{id: connID, stream: stream}
	connStreams = append(connStreams, connStream)
	cmap.sfnCollection[token] = connStreams
	cmap.connSfnMap[connID] = token
}

// Get returns a quic stream which represents stream function connection.
func (cmap *ConcurrentMap) Get(token string) *quic.Stream {
	cmap.l.RLock()
	defer cmap.l.RUnlock()
	if val, ok := cmap.sfnCollection[token]; ok {
		l := len(val)
		if len(val) == 0 {
			logger.Debugf("not available stream, token=%s", token)
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

// GetSfn get the name of stream function.
func (cmap *ConcurrentMap) GetSfn(connID string) (string, bool) {
	cmap.l.RLock()
	defer cmap.l.RUnlock()
	name, ok := cmap.connSfnMap[connID]
	return name, ok
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
					if i+1 < len(connStreams) {
						connStreams = append(connStreams[:i], connStreams[i+1:]...)
					} else {
						connStreams = connStreams[:i]
					}
				}
			}
		}
		cmap.sfnCollection[key] = connStreams
	}
	// remove connection and stream function map
	for _, connID := range connIDs {
		delete(cmap.connSfnMap, connID)
	}
}

// // WriteToAll will dispatch data to all stream functions.
// func (cmap *ConcurrentMap) WriteToAll(val []byte) {
// 	for _, targets := range cmap.sfnCollection {
// 		for _, target := range targets {
// 			(*target.stream).Write(val)
// 		}
// 	}
// }

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

// AddFunc add user stream function to workflow.
func (cmap *ConcurrentMap) AddFunc(index int, name string) {
	cmap.l.Lock()
	defer cmap.l.Unlock()
	cmap.funcBuckets[index] = name
}

// ExistsFunc returns if func by given name is existed.
func (cmap *ConcurrentMap) ExistsFunc(name string) bool {
	cmap.l.RLock()
	defer cmap.l.RUnlock()
	for _, k := range cmap.funcBuckets {
		if k == name {
			return true
		}
	}
	return false
}

// Write will dispatch DataFrame to stream functions. from is the f sent from.
func (cmap *ConcurrentMap) Write(f *frame.DataFrame, from string) error {
	// from is the rem_addr of sfn
	currentIssuer, ok := cmap.GetSfn(from)
	if !ok {
		currentIssuer = "*source*"
	}
	// immutable data stream, route to next sfn
	var j int
	for i, fn := range cmap.funcBuckets {
		// find next sfn
		if fn == currentIssuer {
			j = i + 1
		}
	}

	// execute first one
	if j == 0 {
		logger.Infof("%s1st sfn write to [%s] -> [%s]:", ServerLogPrefix, currentIssuer, cmap.funcBuckets[0])
		targetStream := cmap.Get(cmap.funcBuckets[0])
		if targetStream == nil {
			logger.Debugf("%ssfn[%s] stream is nil", ServerLogPrefix, cmap.funcBuckets[0])
			err := fmt.Errorf("sfn[%s] stream is nil", cmap.funcBuckets[0])
			return err
		}
		_, err := (*targetStream).Write(f.Encode())
		return err
	}

	if len(cmap.funcBuckets[j]) == 0 {
		logger.Debugf("%sno sfn found, drop this data frame", ServerLogPrefix)
		err := errors.New("no sfn found, drop this data frame")
		return err
	}

	targetStream := cmap.Get(cmap.funcBuckets[j])
	logger.Infof("%swill write to: [%s] -> [%s], target is nil:%v", ServerLogPrefix, currentIssuer, cmap.funcBuckets[j], targetStream == nil)
	if targetStream != nil {
		_, err := (*targetStream).Write(f.Encode())
		return err
	}
	return nil
}
