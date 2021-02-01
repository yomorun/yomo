package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/quic"
)

type DataA struct {
	NumA float32 `yomo:"0x11"`
}

var zipperAddr = os.Getenv("YOMO_ZIPPER_ENDPOINT")

func main() {
	if zipperAddr == "" {
		zipperAddr = "localhost:4242"
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

var codec = y3.NewCodec(0x10)

func generateAndSendData(stream quic.Stream) {
	for {
		time.Sleep(100 * time.Millisecond)
		data := DataA{
			NumA: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
		}

		sendingBuf, _ := codec.Marshal(data)

		_, err := stream.Write(sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to yomo-zipper failure with err: %v", data, err)
		} else {
			log.Printf("✅ Emit %v to yomo-zipper", data)
		}
	}
}
