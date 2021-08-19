package decoder

import "context"

// Option is a function that applies a decoder option.
type Option func(o *options)

// options are the options for YoMo-Client.
type options struct {
	ctx            context.Context // WithContext allows to pass a context.
	OnReceivedData func([]byte)    // OnReceivedData is the function which will be triggered when the data is received.
}

// WithReceivedDataFunc sets the function which will be executed when the data is received.
func WithReceivedDataFunc(f func([]byte)) Option {
	return func(o *options) {
		o.OnReceivedData = f
	}
}

// WithContext allows to pass a context.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

func newOptions(opts ...Option) *options {
	options := &options{}

	for _, o := range opts {
		o(options)
	}

	if options.ctx == nil {
		options.ctx = context.Background()
	}

	return options
}

// GetContext gets the context from opts.
func GetContext(opts ...Option) context.Context {
	options := newOptions(opts...)
	if options.ctx != nil {
		return options.ctx
	}
	return context.Background()
}
