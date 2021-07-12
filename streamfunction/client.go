package streamfunction

import (
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
	fnRx StreamfnRx
}

// New a YoMo Stream Function client.
// The "appName" should match the name of functions in workflow.yaml in yomo-server.
func New(appName string) Client {
	c := &clientImpl{
		Impl: client.New(appName, quic.ConnTypeStreamFunction),
		fnRx: newStreamFnRx(),
	}
	return c
}

// Connect to yomo-server.
func (c *clientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &clientImpl{
		cli,
		c.fnRx,
	}, err
}

// Pipe the handler function in Stream Function.
func (c *clientImpl) Pipe(handler func(rxstream rx.Stream) rx.Stream) {
	appendedStream := c.fnRx.GetAppendedStream(c.Readers, handler)

	for item := range appendedStream.Observe() {
		if item.Error() {
			logger.Error("[Stream Function Client] Handler got the error.", "err", item.E)
		} else if item.V != nil {
			if c.Writer == nil {
				logger.Debug("[Stream Function Client] the writer is nil, won't send the data to yomo-server.", "data", item.V)
				continue
			}

			buf, ok := (item.V).([]byte)
			if !ok {
				logger.Debug("[Stream Function Client] the data is not a []byte in RxStream, won't send it to yomo-server.", "data", item.V)
				continue
			}

			// wrap data with framing.
			f := framing.NewPayloadFrame(buf)
			// send data to yomo-server.
			_, err := c.Writer.Write(f.Bytes())
			if err != nil {
				logger.Error("[Stream Function Client] ❌ Send data to yomo-server failed.", "err", err)
			} else {
				logger.Debug("[Stream Function Client] Send frame to yomo-server", "frame", logger.BytesString(f.Bytes()))
			}
		}

	}
}
