package server

import (
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/quic"
)

// SenderClient is the client for YoMo-Server-Sender (formerly Zipper-Sender) to connect the downsteam YoMo-Server-Receiver (formerly Zipper-Receiver) in edge-mesh.
type SenderClient interface {
	client.Client

	// Connect to downsteam  YoMo-Server-Receiver (formerly Zipper-Receiver).
	Connect(ip string, port int) (SenderClient, error)
}

type senderClientImpl struct {
	*client.Impl
}

// NewSender setups the client of YoMo-Server-Sender (formerly Zipper-Sender).
func NewSender(appName string) SenderClient {
	c := &senderClientImpl{
		Impl: client.New(appName, quic.ConnTypeServerSender),
	}
	return c
}

// Connect to downstream YoMo-Server-Receiver in edge-mesh.
func (c *senderClientImpl) Connect(ip string, port int) (SenderClient, error) {
	cli, err := c.BaseConnect(ip, port)
	return &senderClientImpl{
		cli,
	}, err
}
