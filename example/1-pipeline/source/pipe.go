package main

import (
	"io"
	"log"
	"os"
	"time"

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

	written, err := processPipe(os.Stdin, &TagWriter{tag: 0x01, source: source})

	if err != nil {
		log.Printf(">>>> ERR >>>> %v", err)
		source.Close()
	}
	log.Printf("written: %d", written)
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

func processPipe(r io.Reader, w io.Writer) (int64, error) {
	buf := make([]byte, 4)
	for {
		n, e := r.Read(buf)
		if e != nil {
			log.Printf("\n--ERR--r.Read(): %v", e)
			return 0, e
		}
		// emit data
		written, e := w.Write(buf[:n])
		if e != nil {
			return 0, e
		}
		log.Printf("Read: %# x, written: %d", buf[:n], written)
		time.Sleep(100 * time.Millisecond)
	}
}
