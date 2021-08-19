package source

import (
	"errors"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/frame"
)

type Client struct {
	*client.Impl
}

// New a YoMo-Source client.
func New(appName string) *Client {
	c := &Client{
		Impl: client.New(appName, quic.ConnTypeSource),
	}
	return c
}

// Write the data to downstream.
func (c *Client) Write(data []byte) (int, error) {
	if c.Stream == nil {
		return 0, errors.New("[Source] Stream is nil")
	}

	// wrap data with frame.
	frame := frame.NewDataFrame("tid-test")
	// TODO: let users set the "sid".
	frame.SetCarriage(0x10, data)

	return c.Stream.Write(frame)
}

// Connect to YoMo-Zipper.
func (c *Client) Connect(ip string, port int) (*Client, error) {
	cli, err := c.BaseConnect(ip, port)
	return &Client{
		cli,
	}, err
}
