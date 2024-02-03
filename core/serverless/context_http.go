package serverless

import (
	"github.com/yomorun/yomo/serverless"
)

// HTTP is the interface of Context for HTTP request, but it is not implemented in the server side
func (c *Context) HTTP() serverless.HTTP {
	return nil
}

// HTTP is the interface of CronContext for HTTP request, but it is not implemented in the server side
func (c *CronContext) HTTP() serverless.HTTP {
	return nil
}
