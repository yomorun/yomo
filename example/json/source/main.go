package main

import (
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/yomorun/yomo"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func main() {
	// connect to YoMo-Zipper.
	cli, err := yomo.NewSource(yomo.WithName("yomo-source")).Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}
	log.Printf("✅ Connected to YoMo-Zipper")

	defer cli.Close()

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
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
			log.Printf("❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
		} else {
			log.Printf("✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(100 * time.Millisecond)
	}
}
