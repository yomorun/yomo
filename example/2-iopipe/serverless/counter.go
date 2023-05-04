package main

import (
	"log"

	"github.com/yomorun/yomo/serverless"
)

// Handler counts how many bytes received
func Handler(ctx serverless.Context) {
	log.Printf("Got: %d", len(ctx.Data()))
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []uint32 {
	return []uint32{0x01}
}
