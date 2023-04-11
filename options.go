package yomo

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

// Option is a function that applies a YoMo-Client option.
type Option func(o *Options)

// Options are the options for YoMo
type Options struct {
	MeshConfigURL string // meshConfigURL is the URL of edge-mesh config
	ServerOptions []core.ServerOption
	ClientOptions []core.ClientOption

	// TODO: WithWorkflowConfig
	// zipperWorkflowConfig string // Zipper workflow file
}

// WithMeshConfigURL sets the initial edge-mesh config URL for the YoMo-Zipper.
func WithMeshConfigURL(url string) Option {
	return func(o *Options) {
		o.MeshConfigURL = url
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
func WithObserveDataTags(tags ...frame.Tag) Option {
	return func(o *Options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithObserveDataTags(tags...),
		)
	}
}

// WithLogger sets the client logger
func WithLogger(logger *slog.Logger) Option {
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

	return options
}
