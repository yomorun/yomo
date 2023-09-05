package main

import (
	"log"

	"github.com/yomorun/yomo/serverless"
)

// Init is called once when serverless function is started.
func Init() error {
	log.Println("Init counter function")
	return nil
}

// Handler counts how many bytes received.
func Handler(ctx serverless.Context) {
	log.Printf("Got[%#x]: %d\n", ctx.Tag(), len(ctx.Data()))
}

// DataTags describes the type of data this serverless function observed.
func DataTags() []uint32 {
	return []uint32{0x01}
}
