package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

func TestStreamFunction(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction(
		"test-sfn",
		"localhost:9000",
		WithSfnCredential("token:<CREDENTIAL>"),
		WithSfnLogger(ylog.Default()),
		WithSfnQuicConfig(core.DefalutQuicConfig),
		WithSfnTLSConfig(nil),
	)
	sfn.SetObserveDataTags(0x21)

	exit := make(chan struct{})
	time.AfterFunc(time.Second, func() {
		sfn.Close()
		close(exit)
	})

	// set error handler
	sfn.SetErrorHandler(func(err error) {})

	// set handler
	sfn.SetHandler(AsyncHandleFunc(func(ctx serverless.Context) {
		assert.Equal(t, 0x21, ctx.Tag())
		assert.Equal(t, []byte("test"), ctx.Data())
		ctx.Write(0x22, []byte("backflow"))
	}))

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)

	<-exit
}
