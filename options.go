package yomo

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/auth"
	pkgauth "github.com/yomorun/yomo/pkg/auth"
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

func WithTLSConfig(tc *tls.Config) Option {
	return func(o *Options) {
		o.TLSConfig = tc
	}
}

func WithQuicConfig(qc *quic.Config) Option {
	return func(o *Options) {
		o.QuicConfig = qc
	}
}

func WithClientOptions(opts ...core.ClientOption) Option {
	return func(o *Options) {
		o.ClientOptions = opts
	}
}

func WithServerOptions(opts ...core.ServerOption) Option {
	return func(o *Options) {
		o.ServerOptions = opts
	}
}

// WithAuth sets the server authentication method (used by server)
func WithAuth(auth auth.Authentication) Option {
	return func(o *Options) {
		o.ServerOptions = append(
			o.ServerOptions,
			core.WithAuth(auth),
		)
	}
}

// WithAppKeyCredential sets the client credential (used by client): AppKey
func WithAppKeyCredential(appID string, appSecret string) Option {
	return WithCredential(pkgauth.NewAppKeyCredential(appID, appSecret))
}

// WithCredential sets the client credential
func WithCredential(cred auth.Credential) Option {
	return func(o *Options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithCredential(cred),
		)
	}
}

// WithObservedDataTags sets client data tag list.
func WithObservedDataTags(tags ...byte) Option {
	return func(o *Options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithObservedDataTags(tags...),
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
