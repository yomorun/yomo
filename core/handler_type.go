package core

import (
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/serverless"
)

// AsyncHandler is the request-response mode (asnyc)
// type AsyncHandler func(tag uint32, data []byte) (uint32, []byte)
type AsyncHandler func(ctx *serverless.Context)

// PipeHandler is the bidirectional stream mode (blocking).
type PipeHandler func(in <-chan []byte, out chan<- *frame.PayloadFrame)
