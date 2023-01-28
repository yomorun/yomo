package main

import (
	"encoding/binary"
	"log"

	"github.com/yomorun/yomo/core/frame"
)

// Handler will handle the raw data.
func Handler(data []byte) (frame.Tag, []byte) {
	randint := binary.LittleEndian.Uint32(data)
	log.Printf("Generate random uint32: %d (%# x)", randint, data)
	return 0, nil
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []frame.Tag {
	return []frame.Tag{0x01}
}
