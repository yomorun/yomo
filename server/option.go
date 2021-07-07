package server

// Option is a function that applies a YoMo-Server option.
type Option func(o *options)

// options are the options for YoMo-Server.
type options struct {
	meshConfURL string // meshConfURL is the URL of edge-mesh config.
}

// WithMeshConfURL sets the initial edge-mesh config URL for the YoMo-Server.
func WithMeshConfURL(url string) Option {
	return func(o *options) {
		o.meshConfURL = url
	}
}

// newOptions creates a new options for YoMo-Server.
func newOptions(opts ...Option) *options {
	options := &options{}

	for _, o := range opts {
		o(options)
	}

	return options
}
