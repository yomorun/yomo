package serverless

import "io"

type (
	// CancelFunc represents the function for cancellation.
	CancelFunc func()

	// GetFlowFunc represents the function to get flow.
	GetFlowFunc func() (io.ReadWriter, CancelFunc)

	// GetSinkFunc represents the function to get sink.
	GetSinkFunc func() (io.Writer, CancelFunc)
)
