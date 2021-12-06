package main

import (
	"log"
	"time"

	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/net/websocket"
)

func main() {
	origin := "http://localhost/"
	url := "ws://localhost:7000/"
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		log.Println(err)
		// wait 2s for zipper start-up.
		time.Sleep(2 * time.Second)
		// reconnect
		ws, _ = websocket.Dial(url, "", origin)
	}

	for {
		// receive data from client-1.
		var buf = make([]byte, 512)
		var n int
		if n, err = ws.Read(buf); err == nil {
			dataFrame, err := frame.DecodeToDataFrame(buf[:n])
			if err != nil {
				log.Fatalf("Decode data to DataFrame failed, frame=%# x", buf[:n])
			} else {
				log.Printf("Received: %s.\n", dataFrame.GetCarriage())
			}
		}
	}
}
