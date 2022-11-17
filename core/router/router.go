// router package defines the interface of router,
// router manages application's route according to application's metadata.
//
// route manages data subscribers according to their observed data tags.
package router

import (
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// Router is the interface to manage the routes for applications.
type Router interface {
	// Route gets the route
	Route(metadata metadata.Metadata) Route
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
