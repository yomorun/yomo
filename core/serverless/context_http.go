package serverless

import (
	"github.com/yomorun/yomo/serverless"
)

// HTTP is the interface for HTTP request, but it is not implemented in the server side
func (c *Context) HTTP() serverless.HTTP {
	return nil
}
