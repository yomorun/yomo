package main

import (
	"io"
	"log"
	"os"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/client"
)

var zipperAddr = os.Getenv("YOMO_ZIPPER_ENDPOINT")

func main() {
	if zipperAddr == "" {
		zipperAddr = "localhost:9000"
	}
	// connect to yomo-zipper.
	cli, err := client.NewSource("yomo-source").Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-zipper failure with err: %v", err)
		return
	}

	generateAndSendData(cli)
}

func generateAndSendData(stream io.Writer) {
	keys := []byte{0x10, 0x11, 0x12, 0x13, 0x14}

	for {
		for i, key := range keys {
			time.Sleep(100 * time.Millisecond)
			codec := y3.NewCodec(key)
			num := int64(i + 1)
			sendingBuf, _ := codec.Marshal(num)

			_, err := stream.Write(sendingBuf)
			if err != nil {
				log.Printf("Couldn't send buffer with i=%v", num)
			} else {
				log.Print("Sent: ", num)
			}
		}
	}
}
