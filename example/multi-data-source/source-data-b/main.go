package main

import (
	"fmt"
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
	splits := strings.Split(addr, ":")
	if len(splits) != 2 {
		return fmt.Errorf(`❌ The format of url "%s" is incorrect, it should be "host:port", f.e. localhost:9000`, addr)
	}
	host := splits[0]
	port, err := strconv.Atoi(splits[1])

	cli, err := client.NewSource("source-b").Connect(host, port)
	if err != nil {
		panic(err)
	}

	generateAndSendData(cli)

	return nil
}

var codec = y3.NewCodec(0x12)

func generateAndSendData(writer io.Writer) {
	for {
		time.Sleep(200 * time.Millisecond)
		num := rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200

		sendingBuf, _ := codec.Marshal(num)

		_, err := writer.Write(sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to yomo-zipper failure with err: %f", num, err)
		} else {
			log.Printf("✅ Emit %f to yomo-zipper", num)
		}
	}
}
