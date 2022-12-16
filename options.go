package yomo

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

const (
	// DefaultZipperAddr is the default address of downstream zipper.
	DefaultZipperAddr = "localhost:9000"
)

// Option is a function that applies a YoMo-Client option.
type Option func(o *options)

// Options are the options for YoMo
type options struct {
	zipperAddr    string // target Zipper endpoint address
	meshConfigURL string // meshConfigURL is the URL of edge-mesh config
	serverOptions []core.ServerOption
	clientOptions []core.ClientOption
	quicConfig    *quic.Config
	tlsConfig     *tls.Config

	// TODO: WithWorkflowConfig
	// zipperWorkflowConfig string // Zipper workflow file
}

// WithZipperAddr return a new options with ZipperAddr set to addr.
func WithZipperAddr(addr string) Option {
	return func(o *options) {
		o.zipperAddr = addr
	}
}

// WithMeshConfigURL sets the initial edge-mesh config URL for the YoMo-Zipper.
func WithMeshConfigURL(url string) Option {
	return func(o *options) {
		o.meshConfigURL = url
	}
}

// WithTLSConfig sets the TLS configuration for the client.
func WithTLSConfig(tc *tls.Config) Option {
	return func(o *options) {
		o.tlsConfig = tc
	}
}

// WithQuicConfig sets the QUIC configuration for the client.
func WithQuicConfig(qc *quic.Config) Option {
	return func(o *options) {
		o.quicConfig = qc
	}
}

// WithClientOptions returns a new options with opts.
func WithClientOptions(opts ...core.ClientOption) Option {
	return func(o *options) {
		o.clientOptions = opts
	}
}

// WithServerOptions returns a new options with opts.
func WithServerOptions(opts ...core.ServerOption) Option {
	return func(o *options) {
		o.serverOptions = opts
	}
}

// WithAuth sets the server authentication method (used by server)
func WithAuth(name string, args ...string) Option {
	return func(o *options) {
		o.serverOptions = append(
			o.serverOptions,
			core.WithAuth(name, args...),
		)
	}
}

// WithCredential sets the client credential method (used by client)
func WithCredential(payload string) Option {
	return func(o *options) {
		o.clientOptions = append(
			o.clientOptions,
			core.WithCredential(payload),
		)
	}
}

// WithObserveDataTags sets client data tag list.
func WithObserveDataTags(tags ...frame.Tag) Option {
	return func(o *options) {
		o.clientOptions = append(
			o.clientOptions,
			core.WithObserveDataTags(tags...),
		)
	}
}

// WithLogger sets the client logger
func WithLogger(logger *slog.Logger) Option {
	return func(o *options) {
		o.clientOptions = append(
			o.clientOptions,
			core.WithLogger(logger),
		)
	}
}

// newOptions creates a new options for YoMo-Client.
func newOptions(opts ...Option) *options {
	options := &options{}

	for _, o := range opts {
		o(options)
	}

	if options.zipperAddr == "" {
		options.zipperAddr = DefaultZipperAddr
	}

	return options
}
