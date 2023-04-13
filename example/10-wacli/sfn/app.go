package main

import (
	"log"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/yomorun/yomo/core/frame"
)

func Handler(data []byte) (frame.Tag, []byte) {
	// tingo still does not support `encding/json` for parsing json
	// https://tinygo.org/docs/reference/lang-support/stdlib/
	result := gjson.GetBytes(data, "noise")
	if !result.Exists() {
		log.Println("[sfn] no noise data")
		return 0x0, nil
	}
	noise := result.Float()
	log.Printf("[sfn] got 0x33 noise=%v\n", noise)
	return 0x34, []byte(strconv.FormatFloat(noise, 'g', 5, 64))
}

func DataTags() []frame.Tag {
	return []frame.Tag{0x33}
}
