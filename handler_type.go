package yomo

import (
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/serverless"
)

// AsyncHandler represents the request-response mode (async).
// The AsyncHandler receives messages asynchronously in the Handle function, meaning it
// cannot guarantee the order of the messages.
type AsyncHandler interface {
	// Init initializes the handler.
	// The init function may return an error if initialization fails.
	Init() error
	// Handle handles the observed messages.
	Handle(ctx serverless.Context)
}

// AsyncHandleFunc handles the observed messages.
// AsyncHandleFunc implements the AsyncHandler interface and does nothing when the Init function is called.
type AsyncHandleFunc func(ctx serverless.Context)

// Init does nothing.
func (f AsyncHandleFunc) Init() error {
	return nil
}

// Handle calls AsyncHandleFunc itself.
func (f AsyncHandleFunc) Handle(ctx serverless.Context) {
	f(ctx)
}

// PipeHandler is the bidirectional stream mode (blocking).
type PipeHandler interface {
	// Init initializes the handler.
	// The init function may return an error if initialization fails.
	Init() error
	// Handle processes the observed messages, It receives messages from in and responds to out.
	Handle(in <-chan []byte, out chan<- *frame.DataFrame)
}

// PipeHandleFunc handles the observed messages, It receives messages from in and responds to out.
// PipeHandleFunc implements the PipeHandleFunc interface and does nothing when the Init function is called.
type PipeHandleFunc func(in <-chan []byte, out chan<- *frame.DataFrame)

// Init does nothing.
func (f PipeHandleFunc) Init() error {
	return nil
}

// Handle calls AsyncHandleFunc itself.
func (f PipeHandleFunc) Handle(in <-chan []byte, out chan<- *frame.DataFrame) {
	f(in, out)
}
