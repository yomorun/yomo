package yomo

import (
	"fmt"
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
	time.AfterFunc(2*time.Second, func() {
		source.Close()
		close(exit)
	})

	// error handler
	source.SetErrorHandler(func(err error) {})

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

	fmt.Println("-------------source -------------")

	<-exit
}
