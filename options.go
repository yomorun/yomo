package yomo

import (
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/pkg/auth"
)

const (
	// DefaultZipperAddr is the default address of downstream zipper.
	DefaultZipperAddr = "localhost:9000"
	// DefaultZipperListenAddr set default listening port to 9000 and binding to all interfaces.
	DefaultZipperListenAddr = "0.0.0.0:9000"
)

// Option is a function that applies a YoMo-Client option.
type Option func(o *options)

// options are the options for YoMo-Client.
type options struct {
	ZipperAddr string // target Zipper endpoint address
	// ZipperListenAddr     string // Zipper endpoint address
	ZipperWorkflowConfig string // Zipper workflow file
	MeshConfigURL        string // meshConfigURL is the URL of edge-mesh config
	ServerOptions        []core.ServerOption
	ClientOptions        []core.ClientOption
	// Auth                 auth.Authentication
	// Credential           auth.Credential
}

// WithZipperAddr return a new options with ZipperAddr set to addr.
func WithZipperAddr(addr string) Option {
	return func(o *options) {
		o.ZipperAddr = addr
	}
}

// // WithZipperListenAddr return a new options with ZipperListenAddr set to addr.
// func WithZipperListenAddr(addr string) Option {
// 	return func(o *options) {
// 		o.ZipperListenAddr = addr
// 	}
// }

// WithMeshConfigURL sets the initial edge-mesh config URL for the YoMo-Zipper.
func WithMeshConfigURL(url string) Option {
	return func(o *options) {
		o.MeshConfigURL = url
	}
}

func WithClientOptions(opts ...core.ClientOption) Option {
	return func(o *options) {
		o.ClientOptions = opts
	}
}

func WithServerOptions(opts ...core.ServerOption) Option {
	return func(o *options) {
		o.ServerOptions = opts
	}
}

// WithAppKeyAuth sets the server authentication method (used by server): AppKey
func WithAppKeyAuth(appID string, appSecret string) Option {
	return func(o *options) {
		o.ServerOptions = append(
			o.ServerOptions,
			core.WithAuth(auth.NewAppKeyAuth(appID, appSecret)),
		)
	}
}

// WithAppKeyCredential sets the client credential (used by client): AppKey
func WithAppKeyCredential(appID string, appSecret string) Option {
	return func(o *options) {
		o.ClientOptions = append(
			o.ClientOptions,
			core.WithCredential(auth.NewAppKeyCredential(appID, appSecret)),
		)
	}
}

// WithListener sets the server listener
func WithListener(listener Listener) Option {
	return func(o *options) {
		o.ServerOptions = append(
			o.ServerOptions,
			core.WithListener(listener),
		)
	}
}

// newOptions creates a new options for YoMo-Client.
func newOptions(opts ...Option) *options {
	options := &options{}

	for _, o := range opts {
		o(options)
	}

	if options.ZipperAddr == "" {
		options.ZipperAddr = DefaultZipperAddr
	}

	// if options.ZipperListenAddr == "" {
	// 	options.ZipperListenAddr = DefaultZipperListenAddr
	// }

	return options
}
