package server

import (
	"io"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/rx"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(sfns []yomo.GetStreamFunc, reader io.Reader) rx.Stream {
	stream := rx.FromReader(reader)

	for _, sfn := range sfns {
		stream = stream.MergeStreamFunc(sfn)
	}

	return stream
}
