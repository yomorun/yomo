package main

import (
	"log"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/yomorun/yomo/serverless"
)

func Init() error {
	log.Println("[sfn] init")
	return nil
}

func Handler(ctx serverless.Context) {
	// tingo still does not support `encding/json` for parsing json
	// https://tinygo.org/docs/reference/lang-support/stdlib/
	result := gjson.GetBytes(ctx.Data(), "noise")
	if !result.Exists() {
		log.Println("[sfn] no noise data")
		return
	}
	noise := result.Float()
	log.Printf("[sfn] got 0x33 noise=%v\n", noise)
	ctx.Write(0x34, []byte(strconv.FormatFloat(noise, 'g', 5, 64)))
}

func DataTags() []uint32 {
	return []uint32{0x33}
}
