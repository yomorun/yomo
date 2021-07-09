package streamfunction

import (
	"context"

	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/quic"
	"github.com/yomorun/yomo/rx"
)

// Client is the client for YoMo Stream Function.
type Client interface {
	client.Client

	// Connect to YoMo-Server.
	Connect(ip string, port int) (Client, error)

	// Pipe the Handler function.
	Pipe(f func(rxstream rx.Stream) rx.Stream)
}

type clientImpl struct {
	*client.Impl
	rx streamfnRx
}

// New a YoMo Stream Function client.
// The "appName" should match the name of functions in workflow.yaml in yomo-server.
func New(appName string) Client {
	c := &clientImpl{
		Impl: client.New(appName, quic.ConnTypeStreamFunction),
		rx:   newStreamFnRx(),
	}
	return c
}

// Connect to yomo-server.
func (c *clientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &clientImpl{
		cli,
		c.rx,
	}, err
}

// Pipe the handler function in Stream Function.
func (c *clientImpl) Pipe(handler func(rxstream rx.Stream) rx.Stream) {
	appendedStream := c.getAppendedStream(handler)

	for item := range appendedStream.Observe() {
		if item.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", item.E)
		} else if item.V != nil {
			if c.Writer == nil {
				continue
			}

			buf, ok := (item.V).([]byte)
			if !ok {
				logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, won't send it to yomo-server.", "data", item.V)
				continue
			}

			// send data to yomo-server.
			// wrap data with framing.
			f := framing.NewPayloadFrame(buf)
			_, err := c.Writer.Write(f.Bytes())
			if err != nil {
				logger.Error("[Stream Function Client] ❌ Send data to yomo-server failed.", "err", err)
			} else {
				logger.Debug("[Stream Function Client] Send frame to yomo-server", "frame", logger.BytesString(f.Bytes()))
			}
		}

	}
}

// getAppendedStream gets the stream which appending the new data.
func (c *clientImpl) getAppendedStream(handler func(rxstream rx.Stream) rx.Stream) rx.Stream {
	// create a RxStream from io.Reader with decoder.
	rxStream := rx.FromReaderWithDecoder(c.Readers)
	// create a RawStream from the raw bytes in RxStream.
	rawStream := rxStream.RawBytes()
	// create a new stream by running the `Handler` function.
	fnStream := handler(rxStream)

	// https://github.com/ReactiveX/RxGo#connectable-observable
	// rxstream begins to emit items.
	rxStream.Connect(context.Background())

	// zip RawStream and the new stream from 'Handler' function.
	return c.rx.appendNewDataToRawStream(rawStream, fnStream)
}
