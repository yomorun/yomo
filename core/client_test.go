package core

import (
	"context"
	"testing"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/logger"
)

const testaddr = "127.0.0.1:19999"

func Test_Client_Dial_Nothing(t *testing.T) {
	ctx := context.Background()

	client := NewClient("source", ClientTypeSource)

	assert.Equal(t, ConnStateReady, client.State(), "client state should be ConnStateReady")

	err := client.Connect(ctx, testaddr)

	assert.Equal(t, ConnStateDisconnected, client.State(), "client state should be ConnStateDisconnected")

	qerr := &quic.IdleTimeoutError{}
	assert.ErrorAs(t, err, &qerr, "dial must timeout")
}

func Test_Frame_RoundTrip(t *testing.T) {
	ctx := context.Background()

	var (
		obversedTag = frame.Tag(1)
		payload     = []byte("hello data frame")
	)

	server := NewServer("zipper",
		WithAddr(testaddr),
		WithAuth("token", "auth-token"),
		WithServerQuicConfig(DefalutQuicConfig),
	)
	server.ConfigMetadataBuilder(metadata.DefaultBuilder())
	server.ConfigRouter(router.Default([]config.App{{Name: "sfn-1"}}))

	go func() {
		server.ListenAndServe(ctx, testaddr)
	}()

	source := NewClient(
		"source",
		ClientTypeSource,
		WithCredential("token:auth-token"),
		WithObserveDataTags(obversedTag),
		WithClientQuicConfig(DefalutQuicConfig),
		WithClientTLSConfig(nil),
		WithLogger(logger.Default()),
	)

	source.SetBackflowFrameObserver(func(bf *frame.BackflowFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})

	err := source.Connect(ctx, testaddr)
	assert.Nil(t, err, "source connect must be success")
	assert.Equal(t, ConnStateConnected, source.State(), "source state should be ConnStateReady")

	sfn := NewClient("sfn-1", ClientTypeStreamFunction, WithCredential("token:auth-token"), WithObserveDataTags(obversedTag))

	sfn.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})

	err = sfn.Connect(ctx, testaddr)
	assert.Nil(t, err, "sfn connect must be success")
	assert.Equal(t, ConnStateConnected, sfn.State(), "sfn state should be ConnStateReady")

	// wait source and sfn handshake successful (not elegant).
	time.Sleep(time.Second)

	dataFrame := frame.NewDataFrame()
	dataFrame.SetSourceID(source.clientID)
	dataFrame.SetCarriage(obversedTag, payload)

	err = source.WriteFrame(dataFrame)
	assert.Nil(t, err, "source write dataFrame must be success")

	time.Sleep(time.Second)
	// TODO: closing server many times is blocking.
	assert.Nil(t, server.Close(), "server.Close() should not return error")
}
