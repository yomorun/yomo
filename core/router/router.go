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
	Add(id uint64, observeDataTags []uint32, md metadata.M) error
	// Route gets the ID list of connections from the router.
	Route(dataTag uint32, md metadata.M) (connIDs []uint64)
	// Remove removes the route rule from the router.
	Remove(id uint64)
	// Release release the router and removes all the route rules.
	Release()
}

type defaultRouter struct {
	// mu protects data.
	mu sync.RWMutex

	// targets stores the mapping between connID and the target string that conn wanted.
	targets map[uint64]string

	// data stores tag and connID connection.
	// The key is frame tag, The value is connID connection.
	data map[frame.Tag]map[uint64]struct{}
}

// DefaultRouter provides a default implementation of `router`,
// It routes data according to observed tag and metadata.
func Default() *defaultRouter {
	return &defaultRouter{
		targets: make(map[uint64]string),
		data:    make(map[frame.Tag]map[uint64]struct{}),
	}
}

func (r *defaultRouter) Add(connID uint64, observeDataTags []uint32, md metadata.M) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	target, ok := md.Get(metadata.WantedTargetKey)
	if ok {
		r.targets[connID] = target
	}

	for _, tag := range observeDataTags {
		conns := r.data[tag]
		if conns == nil {
			conns = map[uint64]struct{}{}
			r.data[tag] = conns
		}
		r.data[tag][connID] = struct{}{}
	}

	return nil
}

func (r *defaultRouter) Route(dataTag uint32, md metadata.M) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	target, existed := md.Get(metadata.TargetKey)

	var connID []uint64
	if conns, ok := r.data[dataTag]; ok {
		for k := range conns {
			if existed {
				if wt, ok := r.targets[k]; ok && wt == target {
					connID = append(connID, k)
				}
			} else {
				connID = append(connID, k)
			}
		}
	}

	return connID
}

func (r *defaultRouter) Remove(connID uint64) {
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
