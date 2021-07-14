package mock

import (
	"fmt"

	"github.com/yomorun/yomo/server"
)

const (
	IP   string = "127.0.0.1"
	Port int    = 8111
)

// New a mock server.
func New() {
	svr := server.New(&server.WorkflowConfig{})
	svr.Serve(fmt.Sprintf("%s:%d", IP, Port))
}
