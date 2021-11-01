package core

import (
	"sync"

	"github.com/yomorun/yomo/pkg/logger"
)

var _ Router = &router{}

type Router interface {
	Add(index int, name string)
	Next(current string) (string, bool)
	Exists(name string) bool
}

type router struct {
	data sync.Map
}

func newRouter() *router {
	return &router{
		data: sync.Map{},
	}
}

// Add add route
func (r *router) Add(index int, name string) {
	logger.Debugf("%srouter add: %s", ServerLogPrefix, name)
	r.data.Store(index, name)
}

// Exists
func (r *router) Exists(name string) bool {
	var ok bool
	r.data.Range(func(key interface{}, val interface{}) bool {
		if val.(string) == name {
			ok = true
			return false
		}
		return true
	})

	return ok
}

func (r *router) Next(current string) (string, bool) {
	var idx int
	r.data.Range(func(key interface{}, val interface{}) bool {
		if val.(string) == current {
			idx = key.(int) + 1
			return false
		}
		return true
	})
	to, ok := r.data.Load(idx)
	if ok {
		return to.(string), true
	}

	return "", false
}
