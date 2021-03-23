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
		return err
	}

	emitterStream, err := emitterClient.CreateStream(context.Background())
	if err != nil {
		return err
	}

	// Serverless QUIC client.
	host := strings.Split(addr, ":")[0]
	port, err := strconv.Atoi(strings.Split(addr, ":")[1])

	if err != nil {
		return err
	}

	cli, err := client.Connect(host, port).Name("Mock").Writer()

	if err != nil {
		return err
	}

	// Read data from Emitter stream and write to Serverless stream.
	go io.Copy(cli, emitterStream)

	// send data to Emitter server and receive unbounded incoming data afterwards.
	_, err = emitterStream.Write([]byte("ping"))
	if err != nil {
		return err
	}

	return nil
}
