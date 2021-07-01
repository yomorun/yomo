package client

import (
	"context"

	"github.com/yomorun/yomo/pkg/logger"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

// OutputConnectorClient is the client for YoMo Output Connector.
type OutputConnectorClient interface {
	client

	// Connect to YoMo-Server.
	Connect(ip string, port int) (OutputConnectorClient, error)

	// Run the Handler function.
	Run(f func(rxstream rx.RxStream) rx.RxStream)
}

type outputConnClientImpl struct {
	*clientImpl
}

// NewOutputConnector setups the client of YoMo Output Connector.
func NewOutputConnector(appName string) OutputConnectorClient {
	c := &outputConnClientImpl{
		clientImpl: newClient(appName, quic.ConnTypeOutputConnector),
	}
	return c
}

// Connect to yomo-server.
func (c *outputConnClientImpl) Connect(ip string, port int) (OutputConnectorClient, error) {
	cli, err := c.connect(ip, port)
	return &outputConnClientImpl{
		cli,
	}, err
}

// Run the handler function in Output Connector.
func (c *outputConnClientImpl) Run(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithDecoder(c.readers)
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
