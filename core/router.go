package core

type Router interface {
	Route(appID string) Route
	Clean()
}
type Route interface {
	Add(index int, name string)
	Next(current string) (string, bool)
	Exists(name string) bool
}
