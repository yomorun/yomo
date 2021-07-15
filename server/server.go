package server

import (
	"context"

	"github.com/yomorun/yomo/core/quic"
)

// Server represents the YoMo Server.
type Server interface {
	// Serve a YoMo server.
	Serve(endpoint string) error

	// ServeWithHandler serves a YoMo Server with handler.
	ServeWithHandler(endpoint string, handler quic.ServerHandler) error

	// Close the server. All active sessions will be closed.
	Close() error
}

// New a YoMo Server.
func New(conf *WorkflowConfig, opts ...Option) Server {
	options := newOptions(opts...)
	return &serverImpl{
		conf:        conf,
		meshConfURL: options.meshConfURL,
	}
}

type serverImpl struct {
	conf        *WorkflowConfig
	meshConfURL string
	quicServer  quic.Server
}

// Serve a YoMo server.
func (r *serverImpl) Serve(endpoint string) error {
	handler := NewServerHandler(r.conf, r.meshConfURL)
	server := quic.NewServer(handler)
	r.quicServer = server

	// return server.ListenAndServe(context.Background(), endpoint)
	return r.quicServer.ListenAndServe(context.Background(), endpoint)
}

// ServeWithHandler serves a YoMo Server with handler.
func (r *serverImpl) ServeWithHandler(endpoint string, handler quic.ServerHandler) error {
	server := quic.NewServer(handler)
	r.quicServer = server

	return r.quicServer.ListenAndServe(context.Background(), endpoint)
}

// Close the server. All active sessions will be closed.
func (r *serverImpl) Close() error {
	if r.quicServer != nil {
		return r.quicServer.Close()
	}
	return nil
}
