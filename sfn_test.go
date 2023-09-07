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
		"sfn-async-log-events",
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
	sfn.SetHandler(func(ctx serverless.Context) {
		t.Logf("unittest sfn receive <- (%d)", len(ctx.Data()))
		assert.Equal(t, uint32(0x21), ctx.Tag())
		assert.Equal(t, []byte("test"), ctx.Data())
		ctx.Write(0x22, []byte("backflow"))
	})

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)

	<-exit
}

func TestSfnInit(t *testing.T) {
	sfn := NewStreamFunction(
		"test-sfn",
		"localhost:9000",
	)
	var total int64
	err := sfn.Init(func() error {
		total++
		return nil
	})
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
}
