package yomo

import "io"

type (
	CancelFunc func()
	FlowFunc   func() (io.ReadWriter, CancelFunc)
	SinkFunc   func() (io.Writer, CancelFunc)
)
