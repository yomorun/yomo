//go:generate tinygo build -o sfn.wasm -no-debug -target wasi

package main

import (
	"log"

	// "github.com/tidwall/gjson"
	"github.com/buger/jsonparser"
	// "github.com/valyala/fastjson"
	"github.com/yomorun/yomo/core/frame"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func Handler(data []byte) (frame.Tag, []byte) {
	// var model noiseData
	// err := jsoniter.Unmarshal(data, &model)
	result, err := jsonparser.GetFloat(data, "noise")
	// val, err := fastjson.ParseBytes(data)
	// if err != nil {
	// 	log.Fatal("[sfn] parse json error", err)
	// }
	// result := val.GetFloat64("noise")
	if err != nil {
		// if !result.Exists() {
		log.Fatal("[sfn] parse json error", err)
	} else {
		log.Printf("[sfn] got 0x33 noise=%v\n", result)
	}
	return 0x0, nil
}

func DataTags() []frame.Tag {
	return []frame.Tag{0x33}
}
