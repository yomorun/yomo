package core

import "github.com/yomorun/yomo/core/frame"

// Router is the interface to manage the routes for applications.
type Router interface {
	// Route gets the route
	Route(metadata Metadata) Route
	// Clean the routes.
	Clean()
}

// Route manages data subscribers according to their observed data tags.
type Route interface {
	// Add a route.
	Add(connID string, name string, observeDataTags []frame.Tag) error
	// Remove a route.
	Remove(connID string) error
	// GetForwardRoutes returns all the subscribers by the given data tag.
	GetForwardRoutes(tag frame.Tag) []string
}
