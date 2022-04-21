package yomo

import (
	"sync"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/logger"
)

// router
type router struct {
	config *config.WorkflowConfig
}

func newRouter(config *config.WorkflowConfig) core.Router {
	return &router{config: config}
}

// router interface
func (r *router) Route() core.Route {
	logger.Debugf("%sworkflowconfig is %#v", zipperLogPrefix, r.config)
	return newRoute(r.config)
}

func (r *router) Clean() {
	r.config = nil
}

// route interface
type route struct {
	data sync.Map
}

func newRoute(config *config.WorkflowConfig) *route {
	if config == nil {
		logger.Errorf("%sworkflowconfig is nil", zipperLogPrefix)
		return nil
	}
	r := route{
		data: sync.Map{},
	}
	logger.Debugf("%sworkflowconfig %+v", zipperLogPrefix, *config)
	for i, app := range config.Functions {
		r.Add(i, app.Name)
	}

	return &r
}

func (r *route) Add(index int, name string) {
	logger.Debugf("%sroute add: %s", zipperLogPrefix, name)
	r.data.Store(index, name)
}

func (r *route) Exists(name string) bool {
	var ok bool
	logger.Debugf("%srouter[%v] exists name: %s", zipperLogPrefix, r, name)
	r.data.Range(func(key interface{}, val interface{}) bool {
		if val.(string) == name {
			ok = true
			return false
		}
		return true
	})

	return ok
}

func (r *route) GetForwardRoutes(current string) []string {
	idx := -1
	r.data.Range(func(key interface{}, val interface{}) bool {
		if val.(string) == current {
			idx = key.(int)
			return false
		}
		return true
	})

	routes := make([]string, 0)
	r.data.Range(func(key interface{}, val interface{}) bool {
		if key.(int) > idx {
			routes = append(routes, val.(string))
		}
		return true
	})

	return routes
}
