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
	opts := []yomo.Option{}
	if credential := os.Getenv("YOMO_CREDENTIAL"); credential != "" {
		opts = append(opts, yomo.WithCredential(credential))
	}

	source := yomo.NewSource("yomo-source", "localhost:9000", opts...)
	defer source.Close()

	err := source.Connect()
	if err != nil {
		log.Printf("[source] ❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}

	source.SetDataTag(0x33)
	// set the error handler function when server error occurs
	source.SetErrorHandler(func(err error) {
		log.Printf("[source] error handler: %v", err)
	})
	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	generateAndSendData(source)
}

func generateAndSendData(stream yomo.Source) {
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		// encode data via JSON codec.
		sendingBuf, _ := json.Marshal(data)

		// send data via QUIC stream.
		_, err := stream.Write(sendingBuf)
		// using the following code, zipper will broadcast this message to cascading zippers.
		// make sure to configure the downstream zippers using mesh-config flag,
		// see the mesh example for more details.
		// err := stream.Broadcast(sendingBuf)
		if err != nil {
			log.Printf("[source] ❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
		} else {
			log.Printf("[source] ✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
