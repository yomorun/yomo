package outconn

import (
	"context"

	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/quic"
	"github.com/yomorun/yomo/rx"
)

// Client is the client for YoMo Output Connector.
type Client interface {
	client.Client

	// Connect to YoMo-Server.
	Connect(ip string, port int) (Client, error)

	// Run the Handler function.
	Run(f func(rxstream rx.Stream) rx.Stream)
}

type outputConnClientImpl struct {
	*client.Impl
}

// NewClient setups the client of YoMo Output Connector.
func NewClient(appName string) Client {
	c := &outputConnClientImpl{
		Impl: client.New(appName, quic.ConnTypeOutputConnector),
	}
	return c
}

// Connect to yomo-server.
func (c *outputConnClientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &outputConnClientImpl{
		cli,
	}, err
}

// Run the handler function in Output Connector.
func (c *outputConnClientImpl) Run(f func(rxstream rx.Stream) rx.Stream) {
	rxstream := rx.FromReaderWithDecoder(c.Readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	for customer := range stream.Observe() {
		if customer.Error() {
			logger.Error("[Output Connector Client] Handler got the error.", "err", customer.E)
		} else if customer.V != nil {
			logger.Debug("[Output Connector Client] Got the data after ran Handler.", "data", customer.V)
		}
	}
}
