package yomo

import (
	"crypto/tls"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core"
	"golang.org/x/exp/slog"
)

type (
	// SourceOption is option for the Source.
	SourceOption = core.ClientOption

	// SfnOption is option for the SFN.
	SfnOption = core.ClientOption

	// UpstreamZipperOption is option for the upstream Zipper.
	UpstreamZipperOption = core.ClientOption
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

type zipperOptions struct {
	downstreamZipperOption []core.ServerOption
	UpstreamZipperOption   []UpstreamZipperOption
}

// ZipperOption is option for the Zipper.
type ZipperOption func(*zipperOptions)

var (
	// WithAuth sets the zipper authentication method.
	WithAuth = func(name string, args ...string) ZipperOption {
		return func(zo *zipperOptions) {
			zo.downstreamZipperOption = append(zo.downstreamZipperOption, core.WithAuth(name, args...))
		}
	}

	// WithZipperTLSConfig sets the TLS configuration for the zipper.
	WithZipperTLSConfig = func(tc *tls.Config) ZipperOption {
		return func(zo *zipperOptions) {
			zo.downstreamZipperOption = append(zo.downstreamZipperOption, core.WithServerTLSConfig(tc))
		}
	}

	// WithZipperQuicConfig sets the QUIC configuration for the zipper.
	WithZipperQuicConfig = func(qc *quic.Config) ZipperOption {
		return func(zo *zipperOptions) {
			zo.downstreamZipperOption = append(zo.downstreamZipperOption, core.WithServerQuicConfig(qc))
		}
	}

	// WithZipperLogger sets logger for the zipper.
	WithZipperLogger = func(l *slog.Logger) ZipperOption {
		return func(zo *zipperOptions) {
			zo.downstreamZipperOption = append(zo.downstreamZipperOption, core.WithServerLogger(l))
		}
	}

	// WithUptreamOption provides upstream zipper options for Zipper.
	WithUptreamOption = func(opts ...UpstreamZipperOption) ZipperOption {
		return func(o *zipperOptions) {
			o.UpstreamZipperOption = opts
		}
	}
)
