package mocker

import (
	"context"
	"io"

	"github.com/yomorun/yomo/pkg/quic"
)

// EmitMockDataFromCloud emits the mock data from YCloud.
func EmitMockDataFromCloud(addr string) error {
	// Serverless QUIC client.
	slClient, err := quic.NewClient(addr)
	if err != nil {
		return err
	}
	// Emitter QUIC client.
	emitterClient, err := quic.NewClient("emitter.cella.fun:11521")
	if err != nil {
		return err
	}

	// Serverless & Emitter streams.
	slStream, err := slClient.CreateStream(context.Background())
	if err != nil {
		return err
	}
	emitterStream, err := emitterClient.CreateStream(context.Background())
	if err != nil {
		return err
	}

	// Read data from Emitter stream and write to Serverless stream.
	go io.Copy(slStream, emitterStream)

	// send data to Emitter server and receive unbounded incoming data afterwards.
	_, err = emitterStream.Write([]byte("ping"))
	if err != nil {
		return err
	}

	return nil
}
