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
	client := yomo.NewSource("source-pipe", yomo.WithZipperAddr("localhost:9000"))
	defer client.Close()

	// connect to yomo-zipper
	err := client.Connect()
	if err != nil {
		panic(err)
	}

	// set dataID = 0x01
	client.SetDataTag(0x01)

	err = processPipe(os.Stdin, client)

	if err != nil {
		log.Printf(">>>> ERR >>>> %v", err)
		client.Close()
	}

}

func processPipe(r io.Reader, w io.Writer) error {
	buf := make([]byte, 4)
	for {
		_, e := r.Read(buf)
		if e != nil {
			log.Printf("\n--ERR--r.Read(): %v", e)
			return e
		}
		// emit data
		_, e = w.Write(buf)
		if e != nil {
			return e
		}
		log.Printf("Read: %# x", buf)
		time.Sleep(1 * time.Second)
	}
}
