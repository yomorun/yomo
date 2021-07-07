package source

import (
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/quic"
)

// Client is the client for YoMo-Source.
// https://yomo.run/source
type Client interface {
	client.Client

	// Connect to YoMo-Server
	Connect(ip string, port int) (Client, error)
}

type clientImpl struct {
	*client.Impl
}

// New a YoMo-Source client.
func New(appName string) Client {
	c := &clientImpl{
		Impl: client.New(appName, quic.ConnTypeSource),
	}
	return c
}

// Connect to yomo-server.
func (c *clientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &clientImpl{
		cli,
	}, err
}
