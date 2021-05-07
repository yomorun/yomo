package main

import (
	"io"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/client"
)

var zipperAddr = os.Getenv("YOMO_ZIPPER_ENDPOINT")

const charset = "abcdefghijklmnopqrstuvwxyz"

var seed *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func main() {
	if zipperAddr == "" {
		zipperAddr = "localhost:9000"
	}
	err := emit(zipperAddr)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-zipper %s failure with err: %v", zipperAddr, err)
	}
}

func emit(addr string) error {

	host := strings.Split(addr, ":")[0]
	port, err := strconv.Atoi(strings.Split(addr, ":")[1])

	cli, err := client.NewSourceClient("source-b", host, port).Connect()
	if err != nil {
		panic(err)
	}

	generateAndSendData(cli)

	return nil
}

var codec = y3.NewCodec(0x3b)

func generateAndSendData(writer io.Writer) {

	for {
		time.Sleep(200 * time.Millisecond)

		str := generateString()

		sendingBuf, _ := codec.Marshal(str)

		_, err := writer.Write(sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to yomo-zipper failure with err: %s", str, err)
		} else {
			log.Printf("✅ Emit %s to yomo-zipper", str)
		}
	}
}

func generateString() string {
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[seed.Intn(len(charset))]
	}
	return string(b)
}
