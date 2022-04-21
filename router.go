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
func (r *router) Route(appID string) core.Route {
	logger.Debugf("%sapp[%s] workflowconfig is %#v", zipperLogPrefix, appID, r.config)
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
		r.Add(i, app)
	}

	return &r
}

func (r *route) Add(index int, app config.App) {
	logger.Debugf("%sroute add: %s", zipperLogPrefix, app.Name)
	r.data.Store(index, app)
}

func (r *route) Exists(name string) bool {
	var ok bool
	logger.Debugf("%srouter[%v] exists name: %s", zipperLogPrefix, r, name)
	r.data.Range(func(key interface{}, val interface{}) bool {
		if val.(config.App).Name == name {
			ok = true
			return false
		}
		return true
	})

	return ok
}

func (r *route) GetForwardRoutes(current string) []config.App {
	idx := -1
	r.data.Range(func(key interface{}, val interface{}) bool {
		if val.(config.App).Name == current {
			idx = key.(int)
			return false
		}
		return true
	})

	routes := make([]config.App, 0)
	r.data.Range(func(key interface{}, val interface{}) bool {
		if key.(int) > idx {
			routes = append(routes, val.(config.App))
		}
		return true
	})

	return routes
}
