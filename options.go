package yomo

import (
	"github.com/yomorun/yomo/core"
)

type (
	// SourceOption is option for the Source.
	SourceOption = core.ClientOption

	// SfnOption is option for the SFN.
	SfnOption = core.ClientOption
)

var (
	// WithObserveDataTags sets the list of data tags for the Source or SFN.
	WithObserveDataTags = core.WithObserveDataTags

	// WithCredential sets the credential method for the Source or SFN.
	WithCredential = core.WithCredential

	// WithClientTLSConfig sets tls config for the Source or SFN.
	WithClientTLSConfig = core.WithClientTLSConfig

	// WithClientQuicConfig sets quic config for the Source or SFN.
	WithClientQuicConfig = core.WithClientQuicConfig

	// WithLogger sets logger for the Source or SFN.
	WithLogger = core.WithLogger
)

// Option is a function that applies a Zipper option.
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

// NewOptions creates a new options for YoMo-Client.
func NewOptions(opts ...Option) *Options {
	options := &Options{}

	for _, o := range opts {
		o(options)
	}

	return options
}
