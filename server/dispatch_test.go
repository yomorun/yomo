package server

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/internal/framing"
)

// TestDispatcherWithFunc dispatches the input stream to downstreams.
func TestDispatcherWithFunc(t *testing.T) {
	// GetStreamFunc slice
	streamFunc := new(bytes.Buffer)
	getStreamFunc := func() (w io.ReadWriter, cf CancelFunc) {
		return streamFunc, nil
	}
	// reader
	msg := "Clear is better than clever"
	frame := framing.NewPayloadFrame([]byte(msg))
	data := frame.Bytes()
	reader := bytes.NewBuffer(data)
	// dispatch reader to stream functions
	stream := DispatcherWithFunc([]GetStreamFunc{getStreamFunc}, reader)
	// get data from stream
	ch := stream.Observe()
	for item := range ch {
		actual := item.V.([]byte)
		assert.Equal(t, data, actual)
		// frame length: 3
		t.Logf("stream.item: %s\n", framing.GetRawBytesWithoutFraming(actual))
		break
	}
}
