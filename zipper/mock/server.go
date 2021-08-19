package mock

import (
	"fmt"

	server "github.com/yomorun/yomo/zipper"
)

const (
	// IP is the IP of mock server.
	IP string = "127.0.0.1"
	// Port is the Port of mock server.
	Port int = 8111
)

// New a mock server.
func New() {
	svr := server.New(&server.Config{})
	svr.Serve(fmt.Sprintf("%s:%d", IP, Port))
}

// NewWithFuncName creates a mock server with a certain stream-function name.
func NewWithFuncName(funcName string) {
	svr := server.New(&server.Config{
		Functions: []server.Function{
			{
				Name: funcName,
			},
		},
	})
	svr.Serve(fmt.Sprintf("%s:%d", IP, Port))
}
