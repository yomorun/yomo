package client

import "github.com/yomorun/yomo/pkg/quic"

// SourceClient is the client for YoMo-Source.
// https://docs.yomo.run/source
type SourceClient interface {
	client

	// Connect to YoMo-Zipper
	Connect(ip string, port int) (SourceClient, error)
}

type sourceClientImpl struct {
	*clientImpl
}

// NewSource setups the client of YoMo-Source.
func NewSource(appName string) SourceClient {
	c := &sourceClientImpl{
		clientImpl: newClient(appName, quic.ConnTypeSource),
	}
	return c
}

// Connect to yomo-zipper.
func (c *sourceClientImpl) Connect(ip string, port int) (SourceClient, error) {
	cli, err := c.connect(ip, port)
	return &sourceClientImpl{
		cli,
	}, err
}
