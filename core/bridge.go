package core

import "github.com/yomorun/yomo/core/frame"

// Bridge is an interface of bridge which connects the clients of different transport protocols (f.e. WebSocket) with zipper.
type Bridge interface {
	// Name returns the name of bridge.
	Name() string

	// Addr returns the address of bridge.
	Addr() string

	// ListenAndServe starts a server with a given handler.
	ListenAndServe(handler func(ctx *Context)) error

	// Send the data to clients.
	Send(f frame.Frame) error
}
