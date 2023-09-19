// Package router providers a default implement of `router` and `Route`.
package router

import (
	"fmt"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/yerr"
)

// DefaultRouter providers a default implement of `router`,
// It route the data according to observed tag or connID.
type DefaultRouter struct {
	r *defaultRoute
}

// Default return the DefaultRouter.
func Default() Router {
	return &DefaultRouter{r: newRoute()}
}

// Route get route from metadata.
func (r *DefaultRouter) Route(metadata metadata.M) Route {
	return r.r
}

// Clean clean router.
func (r *DefaultRouter) Clean() {
	r.r.mu.Lock()
	defer r.r.mu.Unlock()

	for key := range r.r.data {
		delete(r.r.data, key)
	}
}

type defaultRoute struct {
	data map[frame.Tag]map[string]string
	mu   sync.RWMutex
}

func newRoute() *defaultRoute {
	return &defaultRoute{
		data: make(map[frame.Tag]map[string]string),
		mu:   sync.RWMutex{},
	}
}

func (r *defaultRoute) Add(connID string, name string, observeDataTags []frame.Tag) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

LOOP:
	for _, conns := range r.data {
		for connID, n := range conns {
			if n == name {
				err = yerr.NewDuplicateNameError(connID, fmt.Errorf("SFN[%s] is already linked to another stream", name))
				delete(conns, connID)
				break LOOP
			}
		}
	}

	for _, tag := range observeDataTags {
		conns := r.data[tag]
		if conns == nil {
			conns = make(map[string]string)
			r.data[tag] = conns
		}
		r.data[tag][connID] = name
	}

	return err
}

func (r *defaultRoute) Remove(connID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conns := range r.data {
		delete(conns, connID)
	}

	return nil
}

func (r *defaultRoute) GetForwardRoutes(tag frame.Tag) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var keys []string
	if conns := r.data[tag]; conns != nil {
		for k := range conns {
			keys = append(keys, k)
		}
	}
	return keys
}
