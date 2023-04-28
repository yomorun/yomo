package main

import (
	"encoding/binary"
	"fmt"
	"math"
)

var max float64

// Handler will handle the raw data.
func Handler(data []byte) (uint32, []byte) {
	tmp := binary.BigEndian.Uint64(data)
	val := math.Float64frombits(tmp)

	if val > max {
		max = val
	}

	fmt.Printf("sfn received: %f\n\t💹max: %f\n", val, max)

	return 0x0, nil
}

func DataTags() []uint32 {
	return []uint32{0x34}
}
