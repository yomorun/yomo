package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
)

type SequentialData struct {
	ID int `json:"id"`
}

func main() {
	sfn := yomo.NewStreamFunction("App-2", yomo.WithZipperAddr("localhost:9000"))
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
	data := &SequentialData{}

	// Receive upstream input data sequentially
	for req := range in {
		err := json.Unmarshal(req, data)
		if err != nil {
			log.Fatalln(err)
			continue
		}

		log.Printf("✅ Receive: %d", data.ID)
	}
}
