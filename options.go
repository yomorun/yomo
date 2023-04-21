package yomo

import (
	"crypto/tls"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

type (
	// SourceOption is option for the Source.
	SourceOption core.ClientOption

	// SfnOption is option for the SFN.
	SfnOption core.ClientOption
)

// SourceOption Options.
var (
	// WithObserveDataTags sets the list of data tags for the Source.
	WithObserveDataTags = func(tags ...frame.Tag) SourceOption { return SourceOption(core.WithObserveDataTags(tags...)) }

	// WithCredential sets the credential method for the Source.
	WithCredential = func(payload string) SourceOption { return SourceOption(core.WithCredential(payload)) }

	// WithSourceTLSConfig sets tls config for the Source.
	WithSourceTLSConfig = func(tc *tls.Config) SourceOption { return SourceOption(core.WithClientTLSConfig(tc)) }

	// WithSourceQuicConfig sets quic config for the Source.
	WithSourceQuicConfig = func(qc *quic.Config) SourceOption { return SourceOption(core.WithClientQuicConfig(qc)) }

	// WithLogger sets logger for the Source.
	WithLogger = func(l *slog.Logger) SourceOption { return SourceOption(core.WithLogger(l)) }
)

// Sfn Options.
var (
	// WithSfnCredential sets the credential method for the Sfn.
	WithSfnCredential = func(payload string) SfnOption { return SfnOption(core.WithCredential(payload)) }

	// WithSfnTLSConfig sets tls config for the Sfn.
	WithSfnTLSConfig = func(tc *tls.Config) SourceOption { return SourceOption(core.WithClientTLSConfig(tc)) }

	// WithSfnQuicConfig sets quic config for the Sfn.
	WithSfnQuicConfig = func(qc *quic.Config) SourceOption { return SourceOption(core.WithClientQuicConfig(qc)) }

	// WithSfnLogger sets logger for the Sfn.
	WithSfnLogger = func(l *slog.Logger) SourceOption { return SourceOption(core.WithLogger(l)) }
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
