package framing

// Option is a function that applies a framing option.
type Option func(o *options)

// options are the options for YoMo Frame.
type options struct {
	Metadata []byte
}

// WithMetadata sets the metadata to frame.
func WithMetadata(metadata []byte) Option {
	return func(o *options) {
		o.Metadata = metadata
	}
}

// newOptions creates a new options for Frame.
func newOptions(opts ...Option) *options {
	options := &options{}

	for _, o := range opts {
		o(options)
	}

	return options
}
