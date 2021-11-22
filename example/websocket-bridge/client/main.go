package main

import (
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
		log.Fatal(err)
	}

	// handshake
	credential := auth.NewCredendialNone()
	handshakeFrame := frame.NewHandshakeFrame("ws-bridge-client", byte(core.ClientTypeSource), credential.AppID(), byte(credential.Type()), credential.Payload())
	if _, err := ws.Write(handshakeFrame.Encode()); err != nil {
		log.Fatal(err)
	}

	// data
	for {
		dataFrame := frame.NewDataFrame()
		dataFrame.SetCarriage(0x33, []byte("websocket-bridge"))
		if _, err := ws.Write(dataFrame.Encode()); err != nil {
			log.Fatal(err)
		}

		time.Sleep(1 * time.Second)
	}

	// var msg = make([]byte, 512)
	// var n int
	// if n, err = ws.Read(msg); err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("Received: %s.\n", msg[:n])
}
