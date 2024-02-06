package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

var (
	mockTargetString = "targetString"
)

func TestStreamFunction(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction(
		"sfn-async-log-events",
		"localhost:9000",
		WithSfnCredential("token:<CREDENTIAL>"),
		WithSfnLogger(ylog.Default()),
		WithSfnQuicConfig(core.DefaultClientQuicConfig),
		WithSfnTLSConfig(nil),
	)
	sfn.SetObserveDataTags(0x21)

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set error handler
	sfn.SetErrorHandler(func(err error) {})

	// set handler
	sfn.SetHandler(func(ctx serverless.Context) {
		t.Logf("unittest sfn receive <- (%d)", len(ctx.Data()))
		assert.Equal(t, uint32(0x21), ctx.Tag())
		assert.Equal(t, []byte("test"), ctx.Data())

		err := ctx.WriteWithTarget(0x22, []byte("message from sfn"), mockTargetString)
		assert.Nil(t, err)

	})

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
}

func TestSfnWantedTarget(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction("sfn-handler", "localhost:9000", WithSfnCredential("token:<CREDENTIAL>"))
	sfn.SetObserveDataTags(0x22)
	sfn.SetWantedTarget(mockTargetString)

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set handler
	sfn.SetHandler(func(ctx serverless.Context) {
		t.Logf("unittest handler sfn receive <- (%d)", len(ctx.Data()))
		assert.Equal(t, uint32(0x22), ctx.Tag())
		assert.Contains(t, []string{
			"message from source",
			"message from sfn",
			"message from cron sfn",
		}, string(ctx.Data()))
	})

	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
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

func TestSfnCron(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction("sfn-cron", "localhost:9000", WithSfnCredential("token:<CREDENTIAL>"))

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set cron handler
	sfn.SetCronHandler("@every 200ms", func(ctx serverless.CronContext) {
		t.Log("unittest cron sfn, time reached")
		ctx.Write(0x22, []byte("message from cron sfn"))
		ctx.WriteWithTarget(0x22, []byte("message from cron sfn"), mockTargetString)
	})

	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
}
