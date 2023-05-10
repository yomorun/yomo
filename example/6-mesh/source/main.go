package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/yomorun/yomo"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

var region = os.Getenv("REGION")

func main() {
	// connect to YoMo-Zipper.
	addr := fmt.Sprintf("%s:%d", "localhost", getPort())
	source := yomo.NewSource("yomo-source", addr)
	err := source.Connect()
	if err != nil {
		log.Printf("❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}
	defer source.Close()

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	generateAndSendData(source)
}

func generateAndSendData(stream yomo.Source) {
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  region,
		}

		// Encode data via Y3 codec https://github.com/yomorun/y3-codec.
		sendingBuf, _ := json.Marshal(data)

		// broadcast this message to cascading zippers using `Broadcast` method
		err := stream.Broadcast(0x10, sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
		} else {
			log.Printf("✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(1 * time.Second)
	}
}

func getPort() int {
	port := 9000
	if os.Getenv("PORT") != "" && os.Getenv("PORT") != "9000" {
		port, _ = strconv.Atoi(os.Getenv("PORT"))
	}

	return port
}
