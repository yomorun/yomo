package runtime

import (
	"context"

	"github.com/yomorun/yomo/pkg/quic"
)

// Runtime represents the YoMo runtime.
type Runtime interface {
	// Serve a YoMo server.
	Serve(endpoint string) error
}

// NewRuntime inits a new YoMo runtime.
func NewRuntime(conf *WorkflowConfig, meshConfURL string) Runtime {
	return &runtimeImpl{
		conf:        conf,
		meshConfURL: meshConfURL,
	}
}

type runtimeImpl struct {
	conf        *WorkflowConfig
	meshConfURL string
}

// Serve a YoMo server.
func (r *runtimeImpl) Serve(endpoint string) error {
	handler := NewServerHandler(r.conf, r.meshConfURL)
	server := quic.NewServer(handler)

	return server.ListenAndServe(context.Background(), endpoint)
}
