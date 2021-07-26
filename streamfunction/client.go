package streamfunction

import (
	"context"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// Client is the client for YoMo Stream Function.
type Client interface {
	client.Client

	// Connect to YoMo-Zipper.
	Connect(ip string, port int) (Client, error)

	// Pipe the Handler function.
	// This method is blocking.
	Pipe(handler func(rxstream rx.Stream) rx.Stream)
}

type clientImpl struct {
	*client.Impl
}

// New a YoMo Stream Function client.
// The "appName" should match the name of functions in workflow.yaml in YoMo-Zipper.
func New(appName string) Client {
	c := &clientImpl{
		Impl: client.New(appName, quic.ConnTypeStreamFunction),
	}
	return c
}

// Connect to YoMo-Zipper.
func (c *clientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &clientImpl{
		cli,
	}, err
}

// Pipe the handler function in Stream Function.
// This method is blocking.
func (c *clientImpl) Pipe(handler func(rxstream rx.Stream) rx.Stream) {
	rxstream := rx.NewFactory().FromReaderWithDecoder(c.Readers)
	stream := handler(rxstream)

	rxstream.Connect(context.Background())

	for item := range stream.Observe() {
		if item.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", item.E)
		} else if item.V != nil {
			if c.Writer == nil {
				logger.Debug("[Stream Function Client] the writer is nil, won't send the data to YoMo-Zipper.", "data", item.V)
				continue
			}

			buf, ok := (item.V).([]byte)
			if !ok {
				logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, won't send it to YoMo-Zipper.", "data", item.V)
				continue
			}

			// wrap data with framing.
			frame := framing.NewPayloadFrame(buf)
			// send data to YoMo-Zipper.
			err := c.Writer.Write(frame)
			if err != nil {
				logger.Error("[Stream Function Client] ❌ Send data to YoMo-Zipper failed.", "err", err)
			} else {
				logger.Debug("[Stream Function Client] Send frame to YoMo-Zipper", "frame", logger.BytesString(frame.Bytes()))
			}
		}

	}
}
