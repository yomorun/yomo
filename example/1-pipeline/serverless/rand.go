package main

import (
	"encoding/binary"
	"log"
)

// Handler will handle the raw data.
func Handler(data []byte) (uint32, []byte) {
	randint := binary.LittleEndian.Uint32(data)
	log.Printf("Generate random uint32: %d (%# x)", randint, data)
	return 0, nil
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []uint32 {
	return []uint32{0x01}
}
