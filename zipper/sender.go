package server

import (
	"errors"
	"io"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/framing"
)

// SenderClient is the client for YoMo-Zipper-Sender (formerly Zipper-Sender) to connect the downsteam YoMo-Zipper-Receiver (formerly Zipper-Receiver) in edge-mesh.
type SenderClient interface {
	io.Writer

	client.Client

	// Connect to downsteam  YoMo-Zipper-Receiver (formerly Zipper-Receiver).
	Connect(ip string, port int) (SenderClient, error)
}

type senderClientImpl struct {
	*client.Impl
}

// NewSender setups the client of YoMo-Zipper-Sender (formerly Zipper-Sender).
func NewSender(appName string) SenderClient {
	c := &senderClientImpl{
		Impl: client.New(appName, quic.ConnTypeZipperSender),
	}
	return c
}

// Write the data to downstream.
func (c *senderClientImpl) Write(data []byte) (int, error) {
	if c.Stream == nil {
		return 0, errors.New("[YoMo-Zipper-Sender] Stream is nil")
	}

	// wrap data with frame.
	frame := framing.NewPayloadFrame(data)

	err := c.Stream.Write(frame)
	if err != nil {
		return 0, err
	}

	return len(frame.Bytes()), err
}

// Connect to downstream YoMo-Zipper-Receiver in edge-mesh.
func (c *senderClientImpl) Connect(ip string, port int) (SenderClient, error) {
	cli, err := c.BaseConnect(ip, port)
	return &senderClientImpl{
		cli,
	}, err
}
