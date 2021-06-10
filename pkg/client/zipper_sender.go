package client

import "github.com/yomorun/yomo/pkg/quic"

// ZipperSenderClient is the client for Zipper-Sender to connect the downsteam Zipper-Receiver  in edge-mesh.
type ZipperSenderClient interface {
	client

	// Connect to downsteam Zipper-Receiver
	Connect(ip string, port int) (ZipperSenderClient, error)
}

type zipperSenderClientImpl struct {
	*clientImpl
}

// NewZipperSender setups the client of Zipper-Sender.
func NewZipperSender(appName string) ZipperSenderClient {
	c := &zipperSenderClientImpl{
		clientImpl: newClient(appName, quic.ConnTypeZipperSender),
	}
	return c
}

// Connect to downstream zipper-receiver in edge-mesh.
func (c *zipperSenderClientImpl) Connect(ip string, port int) (ZipperSenderClient, error) {
	cli, err := c.connect(ip, port)
	return &zipperSenderClientImpl{
		cli,
	}, err
}
