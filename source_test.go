package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
)

func TestSource(t *testing.T) {
	t.Parallel()

	source := NewSource(
		"test-source",
		"localhost:9000",
		WithCredential("token:<CREDENTIAL>"),
		WithLogger(ylog.Default()),
		WithObserveDataTags(0x22),
		WithSourceQuicConfig(core.DefalutQuicConfig),
		WithSourceTLSConfig(nil),
	)

	exit := make(chan struct{})
	time.AfterFunc(time.Second, func() {
		source.Close()
		close(exit)
	})

	source.SetErrorHandler(func(err error) {})

	source.SetReceiveHandler(func(tag frame.Tag, data []byte) {
		assert.Equal(t, 0x22, tag)
		assert.Equal(t, []byte("backflow"), data)
	})

	// connect to zipper
	err := source.Connect()
	assert.Nil(t, err)

	// send data to zipper
	err = source.Write(0x21, []byte("test"))
	assert.Nil(t, err)

	// broadcast data to zipper
	err = source.Broadcast(0x21, []byte("test"))
	assert.Nil(t, err)

	<-exit
}
