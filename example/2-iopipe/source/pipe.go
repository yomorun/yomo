package main

import (
	"io"
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	state, _ := os.Stdin.Stat()

	// implement pipe mode, like `cat /dev/urandom | go run pipe.go`
	// check if in pipe mode
	if (state.Mode() & os.ModeCharDevice) != 0 {
		panic("not in pipe, use as `cat /dev/urandom | go run pipe.go`")
	}

	// init yomo-source
	client := yomo.NewSource("source-pipe", "localhost:9000")
	defer client.Close()

	// connect to yomo-zipper
	err := client.Connect()
	if err != nil {
		panic(err)
	}

	// set dataID = 0x01
	client.SetDataTag(0x01)

	written, err := io.Copy(client, os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("written: %d", written)

	select {}
}
