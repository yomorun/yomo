package server

import (
	"context"

	"github.com/yomorun/yomo/core/quic"
)

// Zipper represents the YoMo Zipper.
type Zipper interface {
	// Serve a YoMo Zipper.
	Serve(endpoint string) error

	// ServeWithHandler serves a YoMo Zipper with handler.
	ServeWithHandler(endpoint string, handler quic.ServerHandler) error

	// Close the server. All active sessions will be closed.
	Close() error
}

// New a YoMo Zipper.
func New(conf *WorkflowConfig, opts ...Option) Zipper {
	options := newOptions(opts...)
	return &zipperImpl{
		conf:        conf,
		meshConfURL: options.meshConfURL,
	}
}

type zipperImpl struct {
	conf        *WorkflowConfig
	meshConfURL string
	quicServer  quic.Server
}

// Serve a YoMo Zipper.
func (r *zipperImpl) Serve(endpoint string) error {
	handler := NewServerHandler(r.conf, r.meshConfURL)
	server := quic.NewServer(handler)
	r.quicServer = server

	// return server.ListenAndServe(context.Background(), endpoint)
	return r.quicServer.ListenAndServe(context.Background(), endpoint)
}

// ServeWithHandler serves a YoMo Zipper with handler.
func (r *zipperImpl) ServeWithHandler(endpoint string, handler quic.ServerHandler) error {
	server := quic.NewServer(handler)
	r.quicServer = server

	return r.quicServer.ListenAndServe(context.Background(), endpoint)
}

// Close the server. All active sessions will be closed.
func (r *zipperImpl) Close() error {
	if r.quicServer != nil {
		return r.quicServer.Close()
	}
	return nil
}
