package main

import (
	"fmt"
	"log"
	"time"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/auth"
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

	// handshake
	credential := auth.NewCredendialNone()
	handshakeFrame := frame.NewHandshakeFrame("ws-bridge-client", byte(core.ClientTypeSource), credential.AppID(), byte(credential.Type()), credential.Payload())
	if _, err := ws.Write(handshakeFrame.Encode()); err != nil {
		log.Fatal(err)
	}

	count := 1
	for {
		// send data.
		msg := fmt.Sprintf("websocket-bridge #%d from [client 1]", count)
		dataFrame := frame.NewDataFrame()
		dataFrame.SetCarriage(0x33, []byte(msg))
		if _, err := ws.Write(dataFrame.Encode()); err != nil {
			log.Fatal(err)
		}
		log.Printf("Sent: %s.\n", msg)
		count++

		time.Sleep(1 * time.Second)

		// receive echo data.
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
