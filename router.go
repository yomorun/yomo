package yomo

import (
	"fmt"
	"sync"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/yerr"
	"github.com/yomorun/yomo/pkg/config"
)

type router struct {
	r *route
}

func newRouter(functions []config.App) core.Router {
	return &router{r: newRoute(functions)}
}

func (r *router) Route(metadata core.Metadata) core.Route {
	return r.r
}

func (r *router) Clean() {
	r.r = nil
}

type route struct {
	functions []config.App
	data      map[byte]map[string]string
	mu        sync.RWMutex
}

func newRoute(functions []config.App) *route {
	return &route{
		functions: functions,
		data:      make(map[byte]map[string]string),
	}
}

func (r *route) Add(connID string, name string, observeDataTags []byte) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ok := false
	for _, v := range r.functions {
		if v.Name == name {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("SFN[%s] does not exist in config functions", name)
	}

LOOP:
	for _, conns := range r.data {
		for connID, n := range conns {
			if n == name {
				err = yerr.NewDuplicateNameError(connID, fmt.Errorf("SFN[%s] is already linked to another connection", name))
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

func (r *route) Remove(connID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conns := range r.data {
		delete(conns, connID)
	}

	return nil
}

func (r *route) GetForwardRoutes(tag byte) []string {
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
