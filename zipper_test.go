package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/config"
)

func TestZipperRun(t *testing.T) {
	zipper, err := NewZipper(
		"zipper",
		router.Default(),
		core.DefaultVersionNegotiateFunc,
		map[string]config.Downstream{},
		// WithAuth("token", "<CREDENTIAL>"),
		WithUpstreamOption(core.ClientOption(WithCredential("token:<CREDENTIAL>"))),
		WithZipperLogger(ylog.Default()),
		WithZipperQuicConfig(core.DefaultQuicConfig),
		WithZipperTLSConfig(nil),
	)
	assert.Nil(t, err)
	time.Sleep(time.Second)
	assert.NotNil(t, zipper)
	err = zipper.Close()
	time.Sleep(time.Second)
	assert.Nil(t, err)
}
