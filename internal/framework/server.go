package framework

import (
	"github.com/yomorun/yomo-codec-golang/pkg/codes"
	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"
)

func NewServer(endpoint string, p plugin.YomoObjectPlugin) {
	codec := codes.NewCodec(p.Observed())
	util.QuicServer(endpoint, p, codec)
}
