package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func main() {
	addr := "localhost:9000"
	source := yomo.NewSource(
		"source",
		addr,
	)
	if err := source.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer source.Close()

	sink := yomo.NewStreamFunction(
		"Sink",
		addr,
		yomo.WithSfnTracerProvider(tp),
	)
	sink.SetObserveDataTags(0x34)
	sink.SetHandler(
		func(ctx serverless.Context) {
			log.Printf("[source] received tag[%#x] %s\n", ctx.Tag(), string(ctx.Data()))
		},
	)
	if err := sink.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer sink.Close()

	// set the error handler function when server error occurs
	source.SetErrorHandler(func(err error) {
		log.Printf("[source] error handler: %v", err)
	})
	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	generateAndSendData(source)
}

func generateAndSendData(stream yomo.Source) error {
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
		err := stream.Write(0x33, sendingBuf)
		if err != nil {
			log.Printf("[source] ❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
		} else {
			log.Printf("[source] ✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
