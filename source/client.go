package source

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
)

// Client is the client for YoMo-Source.
// https://docs.yomo.run/source
type Client interface {
	io.Writer

	client.Client

	// Connect to YoMo-Zipper
	Connect(ip string, port int) (Client, error)
}

type clientImpl struct {
	*client.Impl
}

// New a YoMo-Source client.
func New(appName string) Client {
	c := &clientImpl{
		Impl: client.New(appName, core.ConnTypeSource),
	}
	return c
}

// Write the data to downstream.
func (c *clientImpl) Write(data []byte) (int, error) {
	if c.Stream == nil {
		return 0, errors.New("[Source] Stream is nil")
	}

	// wrap data with frame.
	txid := strconv.FormatInt(time.Now().UnixNano(), 10)
	frame := frame.NewDataFrame(frame.NewMetadata("tid", txid))
	// playload frame
	// TODO: tag id
	frame.SetCarriage(0x10, data)

	return c.Stream.WriteFrame(frame)
}

// Connect to YoMo-Zipper.
func (c *clientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &clientImpl{
		cli,
	}, err
}
