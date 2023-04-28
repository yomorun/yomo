package main

import (
	"log"
)

// Handler counts how many bytes received
func Handler(data []byte) (uint32, []byte) {
	log.Printf("Got: %d", len(data))

	// return 0, nil will tell zipper end the workflow.
	return 0, nil
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []uint32 {
	return []uint32{0x01}
}
