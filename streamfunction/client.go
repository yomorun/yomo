package streamfunction

import (
	"context"
	"sync"

	"github.com/reactivex/rxgo/v2"
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
}

// NewClient setups the client of YoMo Stream Function.
// The "appName" should match the name of functions in workflow.yaml in yomo-server.
func NewClient(appName string) Client {
	c := &clientImpl{
		Impl: client.New(appName, quic.ConnTypeStreamFunction),
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

// Pipe the handler function in Stream Function.
func (c *clientImpl) Pipe(f func(rxstream rx.Stream) rx.Stream) {
	// create a RxStream from io.Reader with decoder.
	rxStream := rx.FromReaderWithDecoder(c.Readers)
	// create a RawStream from the raw bytes in RxStream.
	rawStream := rxStream.RawBytes()
	// create a new stream by running the `Handler` function.
	funcStream := f(rxStream)

	// https://github.com/ReactiveX/RxGo#connectable-observable
	// rxstream begins to emit items.
	rxStream.Connect(context.Background())

	// zip RawStream and the new stream from 'Handler' function.
	zippedStream := c.appendNewDataToRawStream(rawStream, funcStream)

	for customer := range zippedStream.Observe() {
		if customer.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", customer.E)
		} else if customer.V != nil {
			if c.Writer == nil {
				continue
			}

			buf, ok := (customer.V).([]byte)
			if !ok {
				logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, won't send it to yomo-server.", "data", customer.V)
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

// appendNewDataToRawStream appends new data to raw stream.
func (c *clientImpl) appendNewDataToRawStream(rawStream rx.Stream, funcStream rx.Stream) rx.Stream {
	opts := []rxgo.Option{
		rxgo.WithErrorStrategy(rxgo.ContinueOnError),
	}

	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		rawData := rawStream.Observe(opts...)
		newData := funcStream.Observe(opts...)
		mutex := sync.Mutex{}
		buf := make([]byte, 0)

		// receive data from raw stream.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case rawItem, ok := <-rawData:
					if !ok {
						return
					}

					if rawItem.Error() {
						logger.Debug("[Stream Function Client] The raw data has an error.", "err", rawItem.E)
						continue
					}

					rawBuf, ok := (rawItem.V).([]byte)
					if !ok {
						logger.Debug("[Stream Function Client] The type of raw data is not []byte.", "rawData", rawItem.V)
						continue
					}
					// append data to buf.
					mutex.Lock()
					buf = append(buf, rawBuf...)
					mutex.Unlock()
				}
			}
		}()

		// receive new data from the stream by `Handler` function.
		for {
			select {
			case <-ctx.Done():
				return
			case newItem, ok := <-newData:
				if !ok {
					return
				}

				mutex.Lock()

				if newItem.Error() {
					logger.Debug("[Stream Function Client] The new data has an error.", "err", newItem.E)
				} else {
					newBuf, ok := (newItem.V).([]byte)
					if !ok {
						logger.Debug("[Stream Function Client] The type of new data is not []byte, won't append it to raw stream.", "newData", newItem.V)
					} else {
						logger.Debug("[Stream Function Client] Append the new data into raw data.", "rawData", logger.BytesString(buf), "newData", logger.BytesString(newBuf))
						// append new data to buf.
						buf = append(buf, newBuf...)
					}
				}

				if len(buf) > 0 {
					// send data to yomo-server.
					rx.Of(buf).SendContext(ctx, next)
					// clean the buf
					buf = make([]byte, 0)
				}

				mutex.Unlock()
			}
		}
	}

	return rx.CreateObservable(f, opts...)
}
