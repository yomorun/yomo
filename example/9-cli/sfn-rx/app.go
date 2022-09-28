package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yomorun/yomo/rx"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

var echo = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	value.From = value.From + ">SFN"
	value.Noise = value.Noise / 10
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[stream-fn] from=%s, Timestamp=%d, value=%f (⚡️=%dms)", value.From, value.Time, value.Noise, rightNow-value.Time))
	// return value.Noise, nil
	return value, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		// Debounce(50).
		Map(echo).
		Marshal(json.Marshal).
		PipeBackToZipper(0x34)

	return stream
}

func DataTags() []byte {
	return []byte{0x33}
}
