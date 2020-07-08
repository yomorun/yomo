package framework

import (
	txtkv "github.com/10cella/yomo-txtkv-codec"
	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"
)

func NewServer(endpoint string, p plugin.YomoObjectPlugin) {
	codec := txtkv.NewCodec(p.Observed())
	util.QuicServer(endpoint, p, codec)
}
