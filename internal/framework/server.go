package framework

import (
	json "github.com/10cella/yomo-json-codec"
	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"
)

func NewServer(endpoint string, p plugin.YomoObjectPlugin) {
	codec := json.NewCodec(p.Observed())
	util.QuicServer(endpoint, p, codec)
}
