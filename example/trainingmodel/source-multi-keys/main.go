package main

import (
	"context"
	"log"
	"os"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/quic"
)

var zipperAddr = os.Getenv("YOMO_ZIPPER_ENDPOINT")

func main() {
	if zipperAddr == "" {
		zipperAddr = "localhost:9999"
	}
	err := emit(zipperAddr)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-zipper %s failure with err: %v", zipperAddr, err)
	}
}

func emit(addr string) error {
	client, err := quic.NewClient(addr)
	if err != nil {
		return err
	}
	log.Printf("✅ Connected to yomo-zipper %s", addr)

	stream, err := client.CreateStream(context.Background())
	if err != nil {
		return err
	}

	generateAndSendData(stream)

	return nil
}

func generateAndSendData(stream quic.Stream) {
	keys := []byte{0x10, 0x11, 0x12, 0x13, 0x14}

	for {
		for i, key := range keys {
			time.Sleep(100 * time.Millisecond)

			codec := y3.NewCodec(key)

			sendingBuf, _ := codec.Marshal(int64(i))

			_, err := stream.Write(sendingBuf)
			if err != nil {
				log.Printf("Couldn't send buffer with i=%v", i)
			} else {
				log.Print(".")
			}
		}
	}
}
