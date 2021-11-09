package store

import "sync"

type MemoryStore struct {
	m sync.Map
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		m: sync.Map{},
	}
}

func (s *MemoryStore) Set(key interface{}, val interface{}) {
	s.m.Store(key, val)
}

func (s *MemoryStore) Get(key interface{}) (interface{}, bool) {
	return s.m.Load(key)
}
