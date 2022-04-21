package core

import "github.com/yomorun/yomo/pkg/config"

// Router is the interface to manage the routes for applications.
type Router interface {
	// Route gets the route by appID.
	Route(appID string) Route
	// Clean the routes.
	Clean()
}

// Route is the interface for route.
type Route interface {
	// Add a route.
	Add(index int, app config.App)
	// GetForwardRoutes returns all the forward routes from current node.
	GetForwardRoutes(current string) []config.App
	// Exists indicates whether the route exists or not.
	Exists(name string) bool
}
