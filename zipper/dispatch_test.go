package server

// import (
// 	"bytes"
// 	"testing"

// 	"github.com/stretchr/testify/assert"
// 	"github.com/yomorun/yomo/internal/decoder"
// 	"github.com/yomorun/yomo/internal/framing"
// )

// // TestDispatcherWithFunc dispatches the input stream to downstreams.
// func TestDispatcherWithFunc(t *testing.T) {
// 	// GetStreamFunc slice
// 	streamFunc := new(bytes.Buffer)
// 	getStreamFunc := func() (n string, w decoder.ReadWriter, cf CancelFunc) {
// 		return "test-fn", decoder.NewReadWriter(streamFunc), nil
// 	}
// 	// reader
// 	msg := "Clear is better than clever"
// 	frame := framing.NewPayloadFrame([]byte(msg))
// 	data := frame.Bytes()
// 	reader := decoder.NewReader(bytes.NewBuffer(data))
// 	// dispatch reader to stream functions
// 	stream := DispatcherWithFunc([]GetStreamFunc{getStreamFunc}, reader)
// 	// get data from stream
// 	ch := stream.Observe()
// 	for item := range ch {
// 		frame := item.V.(framing.Frame)
// 		assert.Equal(t, data, frame.Bytes())
// 		// frame length: 3
// 		t.Logf("stream.item: %s\n", frame.Data())
// 		break
// 	}
// }
