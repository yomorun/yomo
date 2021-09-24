package yomo

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
