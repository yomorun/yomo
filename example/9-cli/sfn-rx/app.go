package main

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/tidwall/gjson"
	"github.com/yomorun/yomo/core/frame"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

var sum float64
var count int

// Handler will handle data in Rx way
func Handler(data []byte) (uint32, []byte) {
	fmt.Printf("sfn received %d bytes: %s\n", len(data), string(data))
	// get noise field from json string
	noiseLevel := gjson.Get(string(data), "noise").Float()

	sum += noiseLevel
	count++

	// calculate average noise level
	avg := sum / float64(count)
	fmt.Printf("\t⚡️avg=%f\n", avg)

	// send result to next processor with data tag=0x34
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(avg))
	return frame.Tag(0x34), buf[:]
}

func DataTags() []uint32 {
	return []uint32{0x33}
}
