package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
)

type SequentialData struct {
	ID int `json:"id"`
}

func main() {
	sfn := yomo.NewStreamFunction("App-1", yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	sfn.SetObserveDataTag(0x33)
	sfn.SetPipeHandler(pipeHandler)

	err := sfn.Connect()
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}

	select {}
}

func pipeHandler(in <-chan []byte, out chan<- *frame.PayloadFrame) {
	rand.Seed(time.Now().UnixNano())
	data := &SequentialData{}

	// Receive upstream input data sequentially
	for req := range in {
		err := json.Unmarshal(req, data)
		if err != nil {
			log.Fatalln(err)
			continue
		}

		// sleep for a random period of time
		sleepTime := rand.Intn(3000)
		log.Printf("✅ Sleep: %d ms", sleepTime)
		time.Sleep(time.Duration(sleepTime) * time.Millisecond)

		// Send data to downstreams
		out <- &frame.PayloadFrame{Tag: 0x33, Carriage: req}
	}
}
