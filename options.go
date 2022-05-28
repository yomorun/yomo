package yomo

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/log"
)

const (
	// DefaultZipperAddr is the default address of downstream zipper.
	DefaultZipperAddr = "localhost:9000"
)

// Option is a function that applies a YoMo-Client option.
type Option func(o *Options)

// Options are the options for YoMo
type Options struct {
	ZipperAddr string // target Zipper endpoint address
	// ZipperListenAddr     string // Zipper endpoint address
	ZipperWorkflowConfig string // Zipper workflow file
	MeshConfigURL        string // meshConfigURL is the URL of edge-mesh config
	ServerOptions        []core.ServerOption
	ClientOptions        []core.ClientOption
	QuicConfig           *quic.Config
	TLSConfig            *tls.Config
	Logger               log.Logger
}

// WithZipperAddr return a new options with ZipperAddr set to addr.
func WithZipperAddr(addr string) Option {
	return func(o *Options) {
		o.ZipperAddr = addr
	}
}

// // WithZipperListenAddr return a new options with ZipperListenAddr set to addr.
// func WithZipperListenAddr(addr string) Option {
// 	return func(o *options) {
// 		o.ZipperListenAddr = addr
// 	}
// }

// TODO: WithWorkflowConfig

// WithMeshConfigURL sets the initial edge-mesh config URL for the YoMo-Zipper.
func WithMeshConfigURL(url string) Option {
	return func(o *Options) {
		o.MeshConfigURL = url
	}
}

// WithTLSConfig sets the TLS configuration for the client.
func WithTLSConfig(tc *tls.Config) Option {
	return func(o *Options) {
		o.TLSConfig = tc
	}
}

// WithQuicConfig sets the QUIC configuration for the client.
func WithQuicConfig(qc *quic.Config) Option {
	return func(o *Options) {
		o.QuicConfig = qc
	}
}

// WithClientOptions returns a new options with opts.
func WithClientOptions(opts ...core.ClientOption) Option {
	return func(o *Options) {
		o.ClientOptions = opts
	}
}

// WithServerOptions returns a new options with opts.
func WithServerOptions(opts ...core.ServerOption) Option {
	return func(o *Options) {
		o.ServerOptions = opts
	}
}

// WithAuth sets the server authentication method (used by server)
func WithAuth(name string, args ...string) Option {
	return func(o *Options) {
		o.ServerOptions = append(
			o.ServerOptions,
			core.WithAuth(name, args...),
		)
	}
}

// WithCredential sets the client credential method (used by client)
func WithCredential(payload string) Option {
	return func(o *Options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithCredential(payload),
		)
	}
}

// WithObserveDataTags sets client data tag list.
func WithObserveDataTags(tags ...byte) Option {
	return func(o *Options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithObserveDataTags(tags...),
		)
	}
}

// WithLogger sets the client logger
func WithLogger(logger log.Logger) Option {
	return func(o *Options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithLogger(logger),
		)
	}
}

// NewOptions creates a new options for YoMo-Client.
func NewOptions(opts ...Option) *Options {
	options := &Options{}

	for _, o := range opts {
		o(options)
	}

	if options.ZipperAddr == "" {
		options.ZipperAddr = DefaultZipperAddr
	}

	return options
}
