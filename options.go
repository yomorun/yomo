package yomo

import (
	"github.com/yomorun/yomo/core"
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

	// WithZipperTLSConfig sets the TLS configuration for the zipper.
	WithZipperTLSConfig = core.WithServerTLSConfig

	// WithZipperQuicConfig sets the QUIC configuration for the zipper.
	WithZipperQuicConfig = core.WithServerQuicConfig

	// WithServerLogger sets logger for the zipper.
	WithZipperLogger = core.WithServerLogger
)

type zipperOptions struct {
	downstreamZipperOption []core.ServerOption
	UpstreamZipperOption   []UpstreamZipperOption
}

// ZipperOption is option for the Zipper.
type ZipperOption func(*zipperOptions)

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
