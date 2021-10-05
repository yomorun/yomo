package main

import (
	"encoding/binary"
	"log"
)

// Handler will handle the raw data.
func Handler(data []byte) (byte, []byte) {
	randint := binary.LittleEndian.Uint32(data)

	log.Printf("Generate random uint32: %d (%# x)", randint, data)

	// return 0, nil will tell zipper end the workflow.
	return 0, nil
}

// DataID describes the type of data this serverless function observed.
func DataID() []byte {
	return []byte{0x01}
}
