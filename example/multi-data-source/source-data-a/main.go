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

var serverAddr = os.Getenv("YOMO_SERVER_ENDPOINT")

func main() {
	if serverAddr == "" {
		serverAddr = "localhost:9000"
	}
	err := emit(serverAddr)
	if err != nil {
		log.Printf("❌ Emit the data to yomo-server %s failure with err: %v", serverAddr, err)
	}
}

func emit(addr string) error {
	splits := strings.Split(addr, ":")
	if len(splits) != 2 {
		return fmt.Errorf(`❌ The format of url "%s" is incorrect, it should be "host:port", f.e. localhost:9000`, addr)
	}
	host := splits[0]
	port, err := strconv.Atoi(splits[1])

	cli, err := client.NewSource("source-a").Connect(host, port)
	if err != nil {
		panic(err)
	}

	defer cli.Close()
	generateAndSendData(cli)

	return nil
}

var codec = y3.NewCodec(0x11)

func generateAndSendData(writer io.Writer) {
	for {
		time.Sleep(200 * time.Millisecond)
		num := rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200

		sendingBuf, _ := codec.Marshal(num)

		_, err := writer.Write(sendingBuf)
		if err != nil {
			log.Printf("❌ Emit %v to yomo-server failure with err: %f", num, err)
		} else {
			log.Printf("✅ Emit %f to yomo-server", num)
		}
	}
}
