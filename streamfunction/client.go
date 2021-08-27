package streamfunction

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/zipper/tracing"
)

// Client is the client for YoMo Stream Function.
type Client interface {
	io.Writer

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
		Impl: client.New(appName, core.ConnTypeStreamFunction),
	}
	return c
}

// Write the data to downstream.
func (c *clientImpl) Write(data []byte) (int, error) {
	if c.Session == nil {
		// the connection was disconnected, retry again.
		c.RetryWithCount(1)
		return 0, errors.New("[Stream Function Client] Session is nil")
	}

	// create a new stream
	stream, err := c.Session.CreateUniStream(context.Background())
	if err != nil {
		return 0, err
	}

	defer stream.Close()

	// tracing
	span := tracing.NewSpanFromData(string(data), "sfn", "sfn-write-to-zipper")
	if span != nil {
		defer span.End()
	}

	return stream.Write(data)
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
	fac := rx.NewFactory()

	for {
		if c.Session == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		quicStream, err := c.Session.AcceptUniStream(context.Background())

		if err != nil {
			if err.Error() != quic.ErrConnectionClosed {
				logger.Error("[Stream Function Client] QUIC Session.AcceptUniStream(ctx) failed.", "err", err)
			}
			continue
		}

		go c.readStreamAndRunHandler(quicStream, handler, fac)
	}
}

// readStreamAndRunHandler reads the QUIC stream from zipper and run `Handler`.
func (c *clientImpl) readStreamAndRunHandler(stream quic.ReceiveStream, handler func(rxstream rx.Stream) rx.Stream, fac rx.Factory) {
	data, err := quic.ReadStream(stream)
	if err != nil {
		logger.Error("[Stream Function Client] receive data from zipper failed.", "err", err)
		return
	}
	// tracing
	span := tracing.NewSpanFromData(string(data), "sfn", "sfn-read-stream-and-run-handler")
	if span != nil {
		defer span.End()
	}

	logger.Debug("[Stream Function Client] received data from zipper.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rxstream := fac.FromItemsWithDecoder([]interface{}{data}, decoder.WithContext(ctx))

	for item := range rxstream.Observe() {
		if item.Error() {
			logger.Error("[Stream Function Client] rxstream got an error.", "err", item.E)
			cancel()
			break
		}

		c.runHandler(ctx, cancel, item.V, handler, fac)
		// one data per time.
		break
	}

}

// runHandler runs the `Handler` and sends the result to zipper if the stream function returns a new data.
func (c *clientImpl) runHandler(ctx context.Context, cancel context.CancelFunc, data interface{}, handler func(rxstream rx.Stream) rx.Stream, fac rx.Factory) {
	defer cancel()
	stream := handler(fac.FromItems(ctx, []interface{}{data}))

	for item := range stream.Observe() {
		if item.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", item.E)
			break
		}

		if item.V == nil {
			logger.Debug("[Stream Function Client] the returned data of Handler is nil.")
			break
		}

		buf, ok := (item.V).([]byte)
		if !ok {
			logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, won't send it to YoMo-Zipper.")
			break
		}

		// send data to YoMo-Zipper.
		_, err := c.Write(buf)
		// logger.Printf("<<<<<<< goroutine %d", runtime.NumGoroutine())
		if err != nil {
			logger.Error("[Stream Function Client] ❌ Send data to YoMo-Zipper failed.", "err", err)
		} else {
			logger.Debug("[Stream Function Client] Send data to YoMo-Zipper.")
		}

		// one data per time.
		break
	}
}
