package runtime

import (
	"io"

	"github.com/yomorun/yomo/pkg/rx"
	"github.com/yomorun/yomo/pkg/serverless"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(flows []serverless.GetFlowFunc, reader io.Reader) rx.RxStream {
	stream := rx.FromReader(reader)

	for _, flow := range flows {
		stream = stream.MergeReadWriterWithFunc(flow)
	}

	return stream
}
