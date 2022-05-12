package core

// Router is the interface to manage the routes for applications.
type Router interface {
	// Route gets the route
	Route(info AppInfo) Route
	// Clean the routes.
	Clean()
}

// Route manages data subscribers according to their observed data tags.
type Route interface {
	// Add a route.
	Add(connID string, name string, observeDataTags []byte) error
	// Remove a route.
	Remove(connID string) error
	// GetForwardRoutes returns all the subscribers by the given data tag.
	GetForwardRoutes(tag byte) []string
}
