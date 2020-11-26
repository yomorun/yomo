package configs

import "github.com/yomorun/yomo/pkg/env"

var (
	DefaultEchoConf EchoConf
)

func init() {
	DefaultEchoConf = GetEchoConf()
}

type EchoConf struct {
	EchoServerAddr string
}

const (
	echoServerAddr = "YOMO_ECHO_SERVER_ADDR"
)

func GetEchoConf() EchoConf {
	conf := EchoConf{}
	conf.EchoServerAddr = env.GetString(echoServerAddr, "161.189.140.133:11521")
	return conf
}
