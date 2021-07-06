package server

import (
	"context"

	"github.com/yomorun/yomo/quic"
)

// Server represents the YoMo Server.
type Server interface {
	// Serve a YoMo server.
	Serve(endpoint string) error
}

// NewServer inits a new YoMo Server.
func NewServer(conf *WorkflowConfig, meshConfURL string) Server {
	return &serverImpl{
		conf:        conf,
		meshConfURL: meshConfURL,
	}
}

type serverImpl struct {
	conf        *WorkflowConfig
	meshConfURL string
}

// Serve a YoMo server.
func (r *serverImpl) Serve(endpoint string) error {
	handler := NewServerHandler(r.conf, r.meshConfURL)
	server := quic.NewServer(handler)

	return server.ListenAndServe(context.Background(), endpoint)
}
