package client

import (
	"context"

	"github.com/yomorun/yomo/pkg/framing"
	"github.com/yomorun/yomo/pkg/logger"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

// StreamFunctionClient is the client for YoMo Stream Function.
type StreamFunctionClient interface {
	client

	// Connect to YoMo-Server.
	Connect(ip string, port int) (StreamFunctionClient, error)

	// Pipe the Handler function.
	Pipe(f func(rxstream rx.RxStream) rx.RxStream)
}

type streamFuncClientImpl struct {
	*clientImpl
}

// NewStreamFunction setups the client of YoMo Stream Function.
// The "appName" should match the name of functions in workflow.yaml in yomo-server.
func NewStreamFunction(appName string) StreamFunctionClient {
	c := &streamFuncClientImpl{
		clientImpl: newClient(appName, quic.ConnTypeStreamFunction),
	}
	return c
}

// Connect to yomo-server.
func (c *streamFuncClientImpl) Connect(ip string, port int) (StreamFunctionClient, error) {
	cli, err := c.connect(ip, port)
	return &streamFuncClientImpl{
		cli,
	}, err
}

// Pipe the handler function in Stream Function.
func (c *streamFuncClientImpl) Pipe(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithDecoder(c.readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	for customer := range stream.Observe() {
		if customer.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", customer.E)
		} else if customer.V != nil {
			if c.writer == nil {
				continue
			}

			buf, ok := (customer.V).([]byte)
			if !ok {
				logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, it won't be sent to yomo-server.", "data", customer.V)
				continue
			}

			// send data to yomo-server.
			// wrap data with framing.
			f := framing.NewPayloadFrame(buf)
			_, err := c.writer.Write(f.Bytes())
			if err != nil {
				logger.Error("❌ Send data to yomo-server failed.", "err", err)
			} else {
				logger.Debug("Send frame to yomo-server", "frame", logger.BytesString(f.Bytes()))
			}
		}

	}
}
