package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
)

func TestZipperRun(t *testing.T) {
	zipper, err := NewZipper(
		"zipper",
		nil,
		// WithAuth("token", "<CREDENTIAL>"),
		WithUpstreamOption(core.ClientOption(WithCredential("token:<CREDENTIAL>"))),
		WithZipperLogger(ylog.Default()),
		WithZipperQuicConfig(core.DefalutQuicConfig),
		WithZipperTLSConfig(nil),
	)
	assert.Nil(t, err)
	time.Sleep(time.Second)
	assert.NotNil(t, zipper)
	err = zipper.Close()
	time.Sleep(time.Second)
	assert.Nil(t, err)
}
