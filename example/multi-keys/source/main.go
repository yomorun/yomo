package main

import (
	"io"
	"log"
	"os"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
)

var serverAddr = os.Getenv("YOMO_SERVER_ENDPOINT")

func main() {
	if serverAddr == "" {
		serverAddr = "localhost:9000"
	}
	// connect to YoMo-Zipper.
	cli, err := yomo.NewSource(yomo.WithName("yomo-source")).Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}

	defer cli.Close()

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
