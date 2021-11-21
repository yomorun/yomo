package store

import (
	"sync"
)

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

func (s *MemoryStore) Remove(key interface{}) {
	s.m.Delete(key)
}

func (s *MemoryStore) Clean() {
	s.m = sync.Map{}
}
