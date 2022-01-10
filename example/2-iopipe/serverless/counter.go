package main

import (
	"log"
)

// Handler counts how many bytes received
func Handler(data []byte) (byte, []byte) {
	log.Printf("Got: %d", len(data))

	// return 0, nil will tell zipper end the workflow.
	return 0, nil
}

// DataID describes the type of data this serverless function observed.
func DataID() []byte {
	return []byte{0x01}
}
