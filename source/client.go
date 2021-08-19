package source

import (
	"errors"
	"io"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/framing"
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
		Impl: client.New(appName, quic.ConnTypeSource),
	}
	return c
}

// Write the data to downstream.
func (c *clientImpl) Write(data []byte) (int, error) {
	if c.Stream == nil {
		return 0, errors.New("[Source] Stream is nil")
	}

	// wrap data with frame.
	frame := framing.NewPayloadFrame(data)

	err := c.Stream.Write(frame)
	if err != nil {
		return 0, err
	}

	return len(frame.Bytes()), err
}

// Connect to YoMo-Zipper.
func (c *clientImpl) Connect(ip string, port int) (Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &clientImpl{
		cli,
	}, err
}
