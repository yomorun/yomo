package yomo

import "io"

type (
	// CancelFunc represents the function for cancellation.
	CancelFunc func()

	// GetStreamFunc represents the function to get stream function (former flow/sink).
	GetStreamFunc func() (io.ReadWriter, CancelFunc)

	// GetSenderFunc represents the function to get YoMo-Sender.
	GetSenderFunc func() (io.Writer, CancelFunc)

	// GetSenderFunc represents the callback function when the specificed key is observed.
	OnObserveFunc func(v []byte) (interface{}, error)
)

// KeyObserveFunc is a pair of subscribed key and onObserve callback.
type KeyObserveFunc struct {
	Key       byte
	OnObserve OnObserveFunc
}
