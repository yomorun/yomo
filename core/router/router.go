// Package router defines the interface of router.
package router

import (
	"fmt"
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

	// data stores tag and connID connection.
	// The key is frame tag, The value is connID connection.
	data map[frame.Tag]map[string]struct{}
}

// DefaultRouter provides a default implementation of `router`,
// It routes data according to observed tag or connID.
func Default() *defaultRouter {
	return &defaultRouter{
		data: make(map[frame.Tag]map[string]struct{}),
	}
}

func (r *defaultRouter) Add(connID string, observeDataTags []uint32, md metadata.M) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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
			target, ok := md.Get("yomo-target")
			fmt.Println("-->", dataTag, ok, target, k, conns)
			if ok {
				if k == target {
					connID = append(connID, k)
				}
			} else {
				connID = append(connID, k)
			}
		}
	}

	return connID
}

func (r *defaultRouter) Remove(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conns := range r.data {
		delete(conns, connID)
	}
}

func (r *defaultRouter) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key := range r.data {
		delete(r.data, key)
	}
}
