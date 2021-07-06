package main

import (
	"log"
	"time"

	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/source"
)

func main() {
	c, err := source.NewClient("cc-src").Connect("localhost", 9000)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-server failure with err: %v", err)
		return
	}
	defer c.Close()

	// The payload to transfer is binary format, value is [0x01, 0x02, 0x03]
	data := []byte{0x01, 0x02, 0x03}
	var obj = y3.NewPrimitivePacketEncoder(0x10)
	// .SetBytes() store value to y3 codec
	obj.SetBytes(data)

	// Finish the data packet to transfer over YoMo
	payload := obj.Encode()
	log.Printf("payload=%v", payload)

	// Write to QUIC Stream
	res, err := c.Write(payload)
	if err != nil {
		log.Printf("❌ c.Write() err: %v", err)
	}

	log.Printf("[Done] c.Write() res=%v", res)

	time.Sleep(30 * time.Second)
	log.Println("exit")
}
