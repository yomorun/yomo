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
	source.SetDataTag(0x33)
	if err := source.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer source.Close()

	sink := yomo.NewStreamFunction(
		"sink",
		addr,
		yomo.WithObserveDataTags(0x34),
	)
	sink.SetHandler(
		func(ctx serverless.Context) {
			log.Printf("[source] received tag[%#x] %s\n", ctx.Tag(), string(ctx.Data()))
		},
	)
	if err := sink.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer sink.Close()

	// send data
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

		sendingBuf, err := json.Marshal(&data)
		if err != nil {
			log.Fatal("json.Marshal error", err)
		}
		// send data via QUIC stream.
		_, err = stream.Write(sendingBuf)

		if err != nil {
			log.Println("[source] ❌ Emit to YoMo-Zipper failure with err ", err, " data", data)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		log.Println("[source] ✅ Emit to YoMo-Zipper", " data", data)
		time.Sleep(1000 * time.Millisecond)
	}
}
