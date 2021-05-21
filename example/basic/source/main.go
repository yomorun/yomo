package main

import (
	"io"
	"log"
	"math/rand"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/client"
)

type noiseData struct {
	Noise float32 `y3:"0x11"` // Noise value
	Time  int64   `y3:"0x12"` // Timestamp (ms)
	From  string  `y3:"0x13"` // Source IP
}

func main() {
	// connect to yomo-zipper.
	cli, err := client.NewSource("yomo-source").Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-zipper failure with err: %v", err)
		return
	}

	defer cli.Close()

	// generate mock data and send it to yomo-zipper in every 100 ms.
	generateAndSendData(cli)
}

var codec = y3.NewCodec(0x10)

func generateAndSendData(stream io.Writer) {
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		// Encode data via Y3 codec https://github.com/yomorun/y3-codec.
		sendingBuf, _ := codec.Marshal(data)

		// send data via QUIC stream.
		_, err := stream.Write(sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to yomo-zipper failure with err: %v", data, err)
		} else {
			log.Printf("✅ Emit %v to yomo-zipper", data)
		}

		time.Sleep(100 * time.Millisecond)
	}
}
