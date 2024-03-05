package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
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
	source := yomo.NewSource(
		"yomo-source",
		"localhost:9000",
	)
	err := source.Connect()
	if err != nil {
		log.Printf("❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}

	defer source.Close()

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	generateAndSendData(source)
}

func generateAndSendData(source yomo.Source) {
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		sendingBuf, err := json.Marshal(data)
		if err != nil {
			log.Fatalln(err)
			os.Exit(-1)
		}

		// send data via QUIC stream.
		err = source.Write(0x10, sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
		} else {
			log.Printf("✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
