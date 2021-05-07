package mocker

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/quic"
)

// EmitMockDataFromCloud emits the mock data from YCloud.
func EmitMockDataFromCloud(addr string) error {
	// Emitter QUIC client.
	emitterClient, err := quic.NewClient("emitter.cella.fun:11521")
	if err != nil {
		panic(err)
	}

	emitterStream, err := emitterClient.CreateStream(context.Background())
	if err != nil {
		panic(err)
	}
	// Serverless QUIC client.
	host := strings.Split(addr, ":")[0]
	port, err := strconv.Atoi(strings.Split(addr, ":")[1])

	if err != nil {
		panic(err)
	}

	cli, err := client.NewSource("Mock").Connect(host, port)
	if err != nil {
		panic(err)
	}
	// Read data from Emitter stream and write to Serverless stream.
	go io.Copy(cli, emitterStream)
	// send data to Emitter server and receive unbounded incoming data afterwards.
	_, err = emitterStream.Write([]byte("ping"))
	if err != nil {
		panic(err)
	}

	return nil
}
