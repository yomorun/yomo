package core

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
	Add(index int, name string)
	// Next gets the next route.
	Next(current string) (string, bool)
	// Exists indicates whether the route exists or not.
	Exists(name string) bool
}
