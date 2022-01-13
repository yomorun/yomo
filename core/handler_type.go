package core

import (
	"github.com/yomorun/yomo/core/frame"
)

// SimpleHandler is the request-response mode (asnyc)
type SimpleHandler func([]byte) (byte, []byte)

// PipeHandler is the bidirectional stream mode (blocking).
type PipeHandler func(in <-chan []byte, out chan<- *frame.PayloadFrame)
