package client

import (
	"context"

	"github.com/yomorun/yomo/pkg/framing"
	"github.com/yomorun/yomo/pkg/logger"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

// ServerlessClient is the client for YoMo-Serverless.
type ServerlessClient interface {
	client

	// Connect to YoMo-Zipper
	Connect(ip string, port int) (ServerlessClient, error)

	// Pipe the Handler function.
	Pipe(f func(rxstream rx.RxStream) rx.RxStream)
}

type serverlessClientImpl struct {
	*clientImpl
}

// NewServerless setups the client of YoMo-Serverless.
// The "appName" should match the name of flows (or sinks) in workflow.yaml in zipper.
func NewServerless(appName string) ServerlessClient {
	c := &serverlessClientImpl{
		clientImpl: newClient(appName, quic.ConnTypeServerless),
	}
	return c
}

// Connect to yomo-zipper.
func (c *serverlessClientImpl) Connect(ip string, port int) (ServerlessClient, error) {
	cli, err := c.connect(ip, port)
	return &serverlessClientImpl{
		cli,
	}, err
}

// Pipe the handler function in flow/sink serverless.
func (c *serverlessClientImpl) Pipe(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithDecoder(c.readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	for customer := range stream.Observe() {
		if customer.Error() {
			panic(customer.E)
		} else if customer.V != nil {
			if c.writer == nil {
				continue
			}

			buf, ok := (customer.V).([]byte)
			if !ok {
				logger.Debug("[Serverless Client] the data is not a []byte in RxStream, it won't be sent to zipper.", "data", customer.V)
				continue
			}

			// send data to zipper
			// wrap data with framing.
			f := framing.NewPayloadFrame(buf)
			_, err := c.writer.Write(f.Bytes())
			if err != nil {
				logger.Error("❌ Send data to zipper failed.", "err", err)
			} else {
				logger.Debug("Send frame to zipper", "frame", logger.BytesString(f.Bytes()))
			}
		}

	}
}
