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

// ClientOption is option for the upstream Zipper.
type ClientOption = core.ClientOption

type zipperOptions struct {
	serverOption []core.ServerOption
	clientOption []ClientOption
}

// ZipperOption is option for the Zipper.
type ZipperOption func(*zipperOptions)

var (
	// WithAuth sets the zipper authentication method.
	WithAuth = func(name string, args ...string) ZipperOption {
		return func(zo *zipperOptions) {
			zo.serverOption = append(zo.serverOption, core.WithAuth(name, args...))
		}
	}

	// WithZipperTLSConfig sets the TLS configuration for the zipper.
	WithZipperTLSConfig = func(tc *tls.Config) ZipperOption {
		return func(zo *zipperOptions) {
			zo.serverOption = append(zo.serverOption, core.WithServerTLSConfig(tc))
		}
	}

	// WithZipperQuicConfig sets the QUIC configuration for the zipper.
	WithZipperQuicConfig = func(qc *quic.Config) ZipperOption {
		return func(zo *zipperOptions) {
			zo.serverOption = append(zo.serverOption, core.WithServerQuicConfig(qc))
		}
	}

	// WithZipperLogger sets logger for the zipper.
	WithZipperLogger = func(l *slog.Logger) ZipperOption {
		return func(zo *zipperOptions) {
			zo.serverOption = append(zo.serverOption, core.WithServerLogger(l))
		}
	}

	// WithUptreamOption provides upstream zipper options for Zipper.
	WithUptreamOption = func(opts ...ClientOption) ZipperOption {
		return func(o *zipperOptions) {
			o.clientOption = opts
		}
	}
)
