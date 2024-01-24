package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
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

	// connect to zipper from source
	err := source.Connect()
	assert.Nil(t, err)

	// send data to zipper from source
	err = source.Write(0x21, []byte("test"))
	assert.Nil(t, err)

	err = source.WritePayload(0x22,
		NewPayload([]byte("message from source")).WithTID(mockTID).WithTarget(mockTargetString),
	)
	assert.Nil(t, err)

	<-exit
}
