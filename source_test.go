package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

func TestSource(t *testing.T) {
	t.Parallel()

	// source
	source := NewSource(
		"test-source",
		"localhost:9000",
		WithCredential("token:<CREDENTIAL>"),
		WithLogger(ylog.Default()),
		WithSourceQuicConfig(core.DefaultClientQuicConfig),
		WithSourceTLSConfig(nil),
	)

	exit := make(chan struct{})
	time.AfterFunc(time.Second, func() {
		source.Close()
		close(exit)
	})

	// error handler
	source.SetErrorHandler(func(err error) {})

	// sfn
	sfn := NewStreamFunction(
		"sfn-test",
		"localhost:9000",
		WithSfnCredential("token:<CREDENTIAL>"),
	)
	sfn.SetObserveDataTags(0x21)
	sfn.SetHandler(func(ctx serverless.Context) {
		assert.Equal(t, []byte("test"), ctx.Data())
	})
	err := sfn.Connect()
	assert.Nil(t, err)

	// connect to zipper from source
	err = source.Connect()
	assert.Nil(t, err)

	// send data to zipper from source
	err = source.Write(0x21, []byte("test"))
	assert.Nil(t, err)

	<-exit
}
