package main

import (
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
	source := yomo.NewSource("source-pipe", "localhost:9000")
	defer source.Close()

	// set stream tag
	source.SetStreamTag("abcdefgh")

	// connect to yomo-zipper
	err := source.Connect()
	if err != nil {
		panic(err)
	}

	written, err := source.WriteFrom(os.Stdin)

	if err != nil {
		log.Printf(">>>> ERR >>>> %v", err)
		source.Close()
	}
	log.Printf("written: %d", written)
}
