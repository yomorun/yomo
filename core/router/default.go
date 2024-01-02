// Package router providers a default implement of `router` and `Route`.
package router

import (
	"sync"

	"github.com/yomorun/yomo/core/frame"
)

type defaultRouter struct {
	// mu protects data.
	mu sync.RWMutex

	// data stores tag and connID connection.
	// The key is frame tag, The value is connID connection.
	data map[frame.Tag]map[string]struct{}
}

// DefaultRouter providers a default implement of `router`,
// It routes data according to observed tag or connID.
func Default() *defaultRouter {
	return &defaultRouter{
		data: make(map[frame.Tag]map[string]struct{}),
	}
}

func (r *defaultRouter) Add(conn *RouteParams) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tag := range conn.ObserveDataTags {
		conns := r.data[tag]
		if conns == nil {
			conns = map[string]struct{}{}
			r.data[tag] = conns
		}
		r.data[tag][conn.ID] = struct{}{}
	}

	return err
}

func (r *defaultRouter) Remove(conn *RouteParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conns := range r.data {
		delete(conns, conn.ID)
	}

	return nil
}

func (r *defaultRouter) Get(conn *RouteParams) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var connID []string
	for _, tag := range conn.ObserveDataTags {
		if conns, ok := r.data[tag]; ok {
			for k := range conns {
				connID = append(connID, k)
			}
		}
	}
	return connID
}

func (r *defaultRouter) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key := range r.data {
		delete(r.data, key)
	}
}
