package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/yomorun/yomo"
)

func main() {
	// connect to YoMo-Zipper.
	cli, err := yomo.NewSource(yomo.WithName("yomo-src-v")).Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}
	defer cli.Close()

	// generate mock data and send it to YoMo-Zipper.
	generateAndSendData(cli)
	time.Sleep(5 * time.Second)
}

func generateAndSendData(stream io.Writer) {
	var i int = 0
	for {
		i++
		buf := []byte(fmt.Sprintf("data-%d", i))

		// send data via QUIC stream.
		n, err := stream.Write(buf)
		if err != nil {
			log.Printf("❌ Emit %# x to YoMo-Zipper failure with err: %v", buf, err)
			break
		}
		log.Printf("✅ stream.write wrote %d bytes", n)

		time.Sleep(1 * time.Second)
	}
}
