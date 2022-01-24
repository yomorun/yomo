package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/yomorun/yomo"
)

type SequentialData struct {
	ID int `json:"id"`
}

func main() {
	source := yomo.NewSource(
		"yomo-source",
		yomo.WithZipperAddr("localhost:9000"),
	)

	err := source.Connect()
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
	defer source.Close()

	source.SetDataTag(0x33)

	id := 0
	for {
		id++
		data := &SequentialData{ID: id}

		sendingBuf, err := json.Marshal(data)
		if err != nil {
			log.Fatalln(err)
			os.Exit(1)
		}

		_, err = source.Write(sendingBuf)
		if err != nil {
			log.Fatalln(err)
			os.Exit(1)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
