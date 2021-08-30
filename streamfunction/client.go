package streamfunction

import (
	"context"
	"errors"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/zipper/tracing"
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
		Impl: client.New(appName, core.ConnTypeStreamFunction),
	}
	return c
}

// Write the data to downstream.
func (c *clientImpl) Write(data *frame.DataFrame) (int, error) {
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
	span := tracing.NewSpanFromData(string(data.GetCarriage()), "sfn", "sfn-write-to-zipper")
	if span != nil {
		defer span.End()
	}

	return stream.Write(data.Encode())
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
	f, err := core.ParseFrame(stream)
	// TODO: Y3 should handle "EOF".
	if err != nil {
		logger.Error("[Stream Function Client] receive data from zipper failed.", "err", err)
		return
	}

	if f.Type() != frame.TagOfDataFrame {
		logger.Debug("[Stream Function Client] YoMo-Zipper received frame from `stream-fn`, but the frame type is not a DataFrame.", "type", f.Type().String())
		return
	}

	dataFrame := f.(*frame.DataFrame)

	// tracing
	span := tracing.NewSpanFromData(string(dataFrame.GetCarriage()), "sfn", "sfn-read-stream-and-run-handler")
	if span != nil {
		defer span.End()
	}

	logger.Debug("[Stream Function Client] received data from zipper.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: remove Rx
	rxstream := fac.FromItemsWithDecoder([]interface{}{dataFrame.GetCarriage()}, decoder.WithContext(ctx))

	for item := range rxstream.Observe() {
		if item.Error() {
			logger.Error("[Stream Function Client] rxstream got an error.", "err", item.E)
			break
		}

		c.runHandler(ctx, item.V, dataFrame, handler, fac)
		// one data per time.
		break
	}

}

// runHandler runs the `Handler` and sends the result to zipper if the stream function returns a new data.
// TODO: remove Rx
func (c *clientImpl) runHandler(ctx context.Context, data interface{}, dataFrame *frame.DataFrame, handler func(rxstream rx.Stream) rx.Stream, fac rx.Factory) {
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
		// TODO: tag id should be set by user.
		dataFrame.SetCarriage(0x13, buf)
		_, err := c.Write(dataFrame)
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
