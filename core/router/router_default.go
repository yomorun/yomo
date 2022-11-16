package router

import (
	"fmt"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/yerr"
	"github.com/yomorun/yomo/pkg/config"
)

type DefaultRouter struct {
	r *DefaultRoute
}

func Default(functions []config.App) Router {
	return &DefaultRouter{r: newRoute(functions)}
}

func (r *DefaultRouter) Route(metadata metadata.Metadata) Route {
	return r.r
}

func (r *DefaultRouter) Clean() {
	r.r = nil
}

type DefaultRoute struct {
	functions []config.App
	data      map[frame.Tag]map[string]string
	mu        sync.RWMutex
}

func newRoute(functions []config.App) *DefaultRoute {
	return &DefaultRoute{
		functions: functions,
		data:      make(map[frame.Tag]map[string]string),
	}
}

func (r *DefaultRoute) Add(connID string, name string, observeDataTags []frame.Tag) (err error) {
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

func (r *DefaultRoute) Remove(connID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conns := range r.data {
		delete(conns, connID)
	}

	return nil
}

func (r *DefaultRoute) GetForwardRoutes(tag frame.Tag) []string {
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
