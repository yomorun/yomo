package main

import (
	"encoding/binary"
	"io"
	"log"
	"time"

	"github.com/yomorun/yomo"
)

func main() {
	sfn := yomo.NewStreamFunction("observer", "127.0.0.1:9000")

	sfn.SetObserveTag("abcdefgh")
	sfn.SetObserveDataTags(1)

	if err := sfn.Connect(); err != nil {
		panic(err)
	}

	sfn.SetObserveHander(func(r io.Reader, _ io.Writer) {
		buf := make([]byte, 4)
		for {
			n, e := r.Read(buf)
			if e != nil {
				log.Printf("\n--ERR--r.Read(): %v", e)
				return
			}

			data := buf[:n]
			randint := binary.LittleEndian.Uint32(data)
			log.Printf("Generate random uint32: %d (%# x)", randint, data)
			time.Sleep(100 * time.Millisecond)
		}
	})

	select {}
}
