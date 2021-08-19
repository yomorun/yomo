package server

import (
	"context"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/utils"
	// "github.com/yomorun/yomo/zipper/tracing"
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
func New(conf *Config, opts ...Option) Zipper {
	options := newOptions(opts...)
	return &zipperImpl{
		conf:        conf,
		meshConfURL: options.meshConfURL,
		logger:      utils.DefaultLogger.WithPrefix("[yomo:zipper]"),
	}
}

type zipperImpl struct {
	conf        *Config
	meshConfURL string
	quicServer  quic.Server
	logger      utils.Logger
}

// Serve start YoMo Zipper.
func (r *zipperImpl) Serve(endpoint string) error {
	r.logger.Debugf("starting zipper ...")
	handler := NewServerHandler(r.conf, r.meshConfURL)
	server := quic.NewServer(handler)
	r.quicServer = server

	// // tracing
	// _, _, err := tracing.NewTracerProvider("zipper")
	// if err != nil {
	// 	log.Println(err)
	// }

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
