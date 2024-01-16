// Package router defines the interface of router.
package router

import (
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// Router routes data that is written by source/sfn according to parameters be passed.
// Users should define their own rules that tells zipper how to route data and how to store the rules.
type Router interface {
	// Add adds the route rule to the router.
	Add(connID string, observeDataTags []uint32, md metadata.M) error
	// Route gets the ID list of connections from the router.
	Route(dataTag uint32, md metadata.M) (connIDs []string)
	// Remove removes the route rule from the router.
	Remove(connID string)
	// Release release the router and removes all the route rules.
	Release()
}

type defaultRouter struct {
	// mu protects data.
	mu sync.RWMutex

	// targets stores the mapping of connID and targetID.
	targets map[string]string

	// data stores tag and connID connection.
	// The key is frame tag, The value is connID connection.
	data map[frame.Tag]map[string]struct{}
}

// DefaultRouter provides a default implementation of `router`,
// It routes data according to observed tag or connID.
func Default() *defaultRouter {
	return &defaultRouter{
		targets: map[string]string{},
		data:    make(map[frame.Tag]map[string]struct{}),
	}
}

func (r *defaultRouter) Add(connID string, observeDataTags []uint32, md metadata.M) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if target, ok := md.Get("yomo-want-target"); ok {
		r.targets[connID] = target
	}

	for _, tag := range observeDataTags {
		conns := r.data[tag]
		if conns == nil {
			conns = map[string]struct{}{}
			r.data[tag] = conns
		}
		r.data[tag][connID] = struct{}{}
	}

	return nil
}

func (r *defaultRouter) Route(dataTag uint32, md metadata.M) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var connID []string
	if conns, ok := r.data[dataTag]; ok {
		for k := range conns {
			if target, ok := r.targets[k]; !ok {
				connID = append(connID, k)
			} else {
				if tt, ok := md.Get("yomo-target"); ok {
					if target == tt {
						connID = append(connID, k)
					}
				}
			}
		}
	}

	return connID
}

func (r *defaultRouter) Remove(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.targets, connID)

	for _, conns := range r.data {
		delete(conns, connID)
	}
}

func (r *defaultRouter) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key := range r.targets {
		delete(r.targets, key)
	}

	for key := range r.data {
		delete(r.data, key)
	}
}
