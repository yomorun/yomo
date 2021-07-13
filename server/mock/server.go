package mock

import (
	"fmt"

	"github.com/yomorun/yomo/server"
)

const (
	ServerIP   string = "127.0.0.1"
	ServerPort int    = 8111
)

// NewServer initializes a new mock server.
func NewServer() {
	svr := server.New(&server.WorkflowConfig{})
	svr.Serve(fmt.Sprintf("%s:%d", ServerIP, ServerPort))
}
