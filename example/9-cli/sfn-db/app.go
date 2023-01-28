package main

import (
	"encoding/json"
	"log"

	"github.com/yomorun/yomo/core/frame"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

// Handler will handle the raw data.
func Handler(data []byte) (frame.Tag, []byte) {
	// var noise float32
	var noise NoiseData
	err := json.Unmarshal(data, &noise)
	if err != nil {
		log.Printf(">> [sink] unmarshal data failed, err=%v", err)
	} else {
		noise.From = noise.From + ">SINK"
		log.Printf(">> [sink] save `%v` to FaunaDB\n", noise)
	}

	return 0x0, nil
}

func DataTags() []frame.Tag {
	return []frame.Tag{0x34}
}
