package main

import (
	"encoding/binary"
	"log"

	"github.com/yomorun/yomo/serverless"
)

// Handler will handle the raw data.
func Handler(ctx serverless.Context) {
	data := ctx.Data()
	randint := binary.LittleEndian.Uint32(data)
	log.Printf("Generate random uint32: %d (%# x)", randint, data)
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []uint32 {
	return []uint32{0x01}
}
