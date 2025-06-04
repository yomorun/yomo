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

	// connect to zipper from source
	err := source.Connect()
	assert.Nil(t, err)

	// send data to zipper from source
	err = source.Write(0x23, []byte("pipe test"))
	assert.Nil(t, err)

	// send data to zipper from source
	err = source.Write(0x21, []byte("test"))
	assert.Nil(t, err)

	err = source.WriteWithTarget(0x22, []byte("message from source"), mockTargetString)
	assert.Nil(t, err)

	source.Write(0xF000, []byte("reserved tag"))

	for {
		err = source.Write(0x24, []byte("try write"))
		if err != nil {
			assert.Error(t, err, "Source: shutdown with error=[0xF000, 0xFFFF] is reserved; please do not write within this range")
			break
		}
		time.Sleep(time.Millisecond * 200)
	}
}
