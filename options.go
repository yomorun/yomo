package yomo

const (
	DefaultZipperAddr       = "localhost:9000"
	DefaultZipperListenAddr = "0.0.0.0:9000"
)

// Option is a function that applies a YoMo-Client option.
type Option func(o *options)

// options are the options for YoMo-Client.
type options struct {
	// AppName              string // AppName is the name of client.
	ZipperAddr           string // Zipper endpoint address
	ZipperListenAddr     string // Zipper endpoint address
	ZipperWorkflowConfig string // Zipper workflow file
}

// WithName sets the initial name for the YoMo-Client.
// func WithName(name string) Option {
// 	return func(o *options) {
// 		o.AppName = name
// 	}
// }

func WithZipperAddr(addr string) Option {
	return func(o *options) {
		o.ZipperAddr = addr
	}
}

func WithZipperListenAddr(addr string) Option {
	return func(o *options) {
		o.ZipperListenAddr = addr
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

	if options.ZipperListenAddr == "" {
		options.ZipperListenAddr = DefaultZipperListenAddr
	}

	return options
}
