package decoder

// Option is a function that applies a decoder option.
type Option func(o *options)

// options are the options for YoMo-Client.
type options struct {
	OnReceivedData func([]byte) // OnReceivedData is the function which will be triggered when the data is received.
}

// WithReceivedDataFunc sets the function which will be executed when the data is received.
func WithReceivedDataFunc(f func([]byte)) Option {
	return func(o *options) {
		o.OnReceivedData = f
	}
}
