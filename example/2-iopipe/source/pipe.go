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
	source := yomo.NewSource("source-pipe", "localhost:9000")
	defer source.Close()

	// connect to yomo-zipper
	err := source.Connect()
	if err != nil {
		panic(err)
	}

	written, err := io.Copy(&TagWriter{tag: 0x01, source: source}, os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("written: %d", written)

	select {}
}

type TagWriter struct {
	tag    uint32
	source yomo.Source
}

func (w *TagWriter) Write(data []byte) (int, error) {
	err := w.source.Write(w.tag, data)
	if err != nil {
		return 0, err
	}
	return len(data), err
}
