// Package router defines the interface of router.
package router

import (
	"github.com/yomorun/yomo/core/metadata"
)

// Router routes data that is written by source/sfn according to RouteParams.
// Users should define their own rule that tells zipper how to route data.
type Router interface {
	// Add adds the route rule to the router.
	Add(*RouteParams) error
	// Get gets the ID of connections from the router.
	Get(*RouteParams) (connIDs []string)
	// Remove removes the route rule from the router.
	Remove(*RouteParams) error
	// Release release the router and removes all the route rules.
	Release()
}

// RouteParams defines the route parameters,
// users defines route rule according to these parameters.
type RouteParams struct {
	// Name is the name of the connection.
	Name string
	// ID is the ID of the connection.
	ID string
	// Metadata is the metadata of the connection.
	Metadata metadata.M
	// ObserveDataTags is the list of data tags that connection observed.
	ObserveDataTags []uint32
}
