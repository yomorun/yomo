package main

import (
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/yomorun/yomo/pkg/client"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func main() {
	// connect to yomo-zipper.
	cli, err := client.NewSource("yomo-source").Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-zipper failure with err: %v", err)
	}
	log.Printf("✅ Connected to yomo-zipper")

	// generate mock data and send it to yomo-zipper in every 100 ms.
	generateAndSendData(cli)
}

func generateAndSendData(stream io.Writer) {
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		// Encode data via JSON.
		sendingBuf, _ := json.Marshal(data)

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
