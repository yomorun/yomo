package core

import (
	"github.com/yomorun/yomo/core/frame"
)

type SimpleHandler func([]byte) (byte, []byte)
type PipeHandler func(in <-chan []byte, out chan<- *frame.PayloadFrame)
