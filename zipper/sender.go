package zipper

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
)

// SenderClient is the client for Upstream YoMo-Zipper (formerly Zipper-Sender) to connect the downsteam YoMo-Zipper (formerly Zipper-Receiver) in edge-mesh.
type SenderClient interface {
	io.Writer

	client.Client

	// Connect to downsteam YoMo-Zipper (formerly Zipper-Receiver).
	Connect(ip string, port int) (SenderClient, error)
}

type senderClientImpl struct {
	*client.Impl
}

// NewSender setups the client of Upstream YoMo-Zipper (formerly Zipper-Sender).
func NewSender(appName string) SenderClient {
	c := &senderClientImpl{
		Impl: client.New(appName, core.ClientTypeUpstreamZipper),
	}
	return c
}

// Write the data to downstream.
func (c *senderClientImpl) Write(data []byte) (int, error) {
	if c.Stream == nil {
		return 0, errors.New("[Upstream YoMo-Zipper] Stream is nil")
	}

	// wrap data with frame.
	txid := strconv.FormatInt(time.Now().UnixNano(), 10)
	frame := frame.NewDataFrame(txid)
	// TODO: tag id
	frame.SetCarriage(0x11, data)

	return c.Stream.WriteFrame(frame)
}

// Connect to downstream YoMo-Zipper in edge-mesh.
func (c *senderClientImpl) Connect(ip string, port int) (SenderClient, error) {
	cli, err := c.BaseConnect(ip, port)
	return &senderClientImpl{
		cli,
	}, err
}
