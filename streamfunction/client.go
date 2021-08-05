package streamfunction

import (
	"context"
	"errors"
	"io"
	"runtime"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
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
		Impl: client.New(appName, quic.ConnTypeStreamFunction),
	}
	return c
}

// Write the data to downstream.
func (c *clientImpl) Write(data []byte) (int, error) {
	if c.Session == nil {
		return 0, errors.New("[Stream Function Client] Session is nil")
	}

	// create a new stream
	stream, err := c.Session.CreateUniStream(context.Background())
	if err != nil {
		return 0, err
	}

	// wrap data with frame.
	frame := framing.NewPayloadFrame(data)

	n, err := stream.Write(frame.Bytes())
	if err != nil {
		stream.Close()
		return 0, err
	}

	// close stream
	go func() {
		time.AfterFunc(time.Second, func() {
			stream.Close()
		})
	}()

	return n, err
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

LOOP_ACCP_STREAM:
	for {
		quicStream, err := c.Session.AcceptUniStream(context.Background())

		if err != nil {
			logger.Error("[Stream Function Client] QUIC Session.AcceptUniStream(ctx) failed.", "err", err)
			break LOOP_ACCP_STREAM
		}

		go c.readStream(quicStream, handler, fac)
	}
}

func (c *clientImpl) readStream(stream quic.ReceiveStream, handler func(rxstream rx.Stream) rx.Stream, fac rx.Factory) {
	reader := decoder.NewReader(stream)
	frameCh := reader.Read()
	for frame := range frameCh {
		data := frame.Data()
		if len(data) == 0 {
			break
		}

		logger.Debug("Received data from zipper.")

		rxstream := fac.FromItemsWithDecoder(data)

		for item := range rxstream.Observe(rxgo.WithObservationStrategy(rxgo.ObservationStrategy(rxgo.CloseChannel))) {
			if item.Error() {
				logger.Error("[Stream Function Client] rxstream got an error.", "err", item.E)
				continue
			}

			go c.executeHandler(item.V, handler, fac)
			// one data per time.
			break
		}

		// one data per time.
		break
	}
}

func (c *clientImpl) executeHandler(data interface{}, handler func(rxstream rx.Stream) rx.Stream, fac rx.Factory) {
	stream := handler(fac.FromItems(data))

	for item := range stream.Observe(rxgo.WithObservationStrategy(rxgo.ObservationStrategy(rxgo.CloseChannel))) {
		if item.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", item.E)
			continue
		}

		if item.V == nil {
			logger.Debug("[Stream Function Client] the returned data of Handler is nil.")
			continue
		}

		buf, ok := (item.V).([]byte)
		if !ok {
			logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, won't send it to YoMo-Zipper.")
			continue
		}

		// send data to YoMo-Zipper.
		_, err := c.Write(buf)
		logger.Print("<<<<<<< goroutine", runtime.NumGoroutine())
		if err != nil {
			logger.Error("[Stream Function Client] ❌ Send data to YoMo-Zipper failed.", "err", err)
		} else {
			logger.Debug("[Stream Function Client] Send frame to YoMo-Zipper.")
		}

		// one data per time.
		break
	}
}
