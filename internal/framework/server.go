package framework

import (

	//ycd "github.com/10cella/yomo-json-codec"

	"github.com/yomorun/yomo-codec-golang/pkg/codes"
	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"
)

func NewServer(endpoint string, p plugin.YomoObjectPlugin) {
	codec := codes.NewCodec(p.Observed())
	//fmt.Printf("#50 codec.Observe=%v\n", codec.Observe)
	util.QuicServer(endpoint, p, codec)
}
