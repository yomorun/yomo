package yomo

import (
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/pkg/config"
)

type (
	// SourceOption is option for the Source.
	SourceOption = core.ClientOption

	// SfnOption is option for the SFN.
	SfnOption = core.ClientOption

	// UpstreamZipperOption is option for the upstream Zipper.
	UpstreamZipperOption = core.ClientOption

	// DownstreamZipperOption is option for the downstream Zipper.
	DownstreamZipperOption = core.ServerOption
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

var (
	// WithAuth sets the zipper authentication method.
	WithAuth = core.WithAuth

	// WithServerTLSConfig sets the TLS configuration for the zipper.
	WithServerTLSConfig = core.WithServerTLSConfig

	// WithServerQuicConfig sets the QUIC configuration for the zipper.
	WithServerQuicConfig = core.WithServerQuicConfig

	// WithServerLogger sets logger for the zipper.
	WithServerLogger = core.WithServerLogger
)

type zipperOptions struct {
	// TODO: meshConfigURL implements MeshConfigProvider interface.
	meshConfigURL          string
	meshConfigProvider     MeshConfigProvider
	downstreamZipperOption []core.ServerOption
	UpstreamZipperOption   []UpstreamZipperOption
}

// ZipperOption is option for the Zipper.
type ZipperOption func(*zipperOptions)

// WithMeshConfigURL sets mesh config url for Zipper.
func WithMeshConfigURL(url string) ZipperOption {
	return func(o *zipperOptions) {
		o.meshConfigURL = url
	}
}

// WithDownstreamOption provides downstream zipper options for Zipper.
func WithDownstreamOption(opts ...DownstreamZipperOption) ZipperOption {
	return func(o *zipperOptions) {
		o.downstreamZipperOption = opts
	}
}

// WithUptreamOption provides upstream zipper options for Zipper.
func WithUptreamOption(opts ...UpstreamZipperOption) ZipperOption {
	return func(o *zipperOptions) {
		o.UpstreamZipperOption = opts
	}
}

// WithMeshConfig
func WithMeshConfigProvider(provider MeshConfigProvider) ZipperOption {
	return func(o *zipperOptions) {
		o.meshConfigProvider = provider
	}
}

// MeshConfigProvider provides the config of mesh zipper.
type MeshConfigProvider interface {
	// Provide returns the config of mesh zipper.
	Provide() []config.MeshZipper
}

type defaultMeshConfigProvider struct {
	confs []config.MeshZipper
}

func (p *defaultMeshConfigProvider) Provide() []config.MeshZipper {
	return p.confs
}

// DefaultMeshConfigProvider returns the config of mesh zipper.
func DefaultMeshConfigProvider(confs ...config.MeshZipper) MeshConfigProvider {
	return &defaultMeshConfigProvider{confs}
}
