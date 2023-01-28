package main

import (
	"log"

	"github.com/yomorun/yomo/core/frame"
)

// Handler counts how many bytes received
func Handler(data []byte) (frame.Tag, []byte) {
	log.Printf("Got: %d", len(data))

	// return 0, nil will tell zipper end the workflow.
	return 0, nil
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []frame.Tag {
	return []frame.Tag{0x01}
}
