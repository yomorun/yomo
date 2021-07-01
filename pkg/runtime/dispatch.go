package runtime

import (
	"io"

	"github.com/yomorun/yomo/pkg/rx"
	"github.com/yomorun/yomo/pkg/serverless"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(sfns []serverless.GetStreamFunc, reader io.Reader) rx.RxStream {
	stream := rx.FromReader(reader)

	for _, sfn := range sfns {
		stream = stream.MergeStreamFunc(sfn)
	}

	return stream
}
