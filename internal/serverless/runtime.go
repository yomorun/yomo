package serverless

import (
	"context"

	"github.com/yomorun/yomo/pkg/quic"
)

// Run serves the Serverless function
func Run(endpoint string, handle quic.ServerHandler) error {
	server := quic.NewServer(handle)

	return server.ListenAndServe(context.Background(), endpoint)
}
