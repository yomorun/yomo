package core

import (
	"sync"

	"github.com/lucas-clemente/quic-go"
)

type ConcurrentMap struct {
	l             sync.RWMutex
	sfnCollection map[string]*quic.Stream
}

func NewConcurrentMap() *ConcurrentMap {
	return &ConcurrentMap{
		sfnCollection: make(map[string]*quic.Stream),
	}
}

func (cmap *ConcurrentMap) Set(key string, val *quic.Stream) {
	cmap.l.Lock()
	defer cmap.l.Unlock()
	cmap.sfnCollection[key] = val
}

func (cmap *ConcurrentMap) Get(key string) *quic.Stream {
	cmap.l.RLock()
	defer cmap.l.RUnlock()
	return cmap.sfnCollection[key]
}

func (cmap *ConcurrentMap) Remove(key string) {
	cmap.l.Lock()
	defer cmap.l.Unlock()
	delete(cmap.sfnCollection, key)
}

func (cmap *ConcurrentMap) WriteToAll(val []byte) {
	for _, target := range cmap.sfnCollection {
		(*target).Write(val)
	}
}

func (cmap *ConcurrentMap) GetCurrentSnapshot() map[string]*quic.Stream {
	return cmap.sfnCollection
}
