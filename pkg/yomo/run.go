package yomo

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/yomorun/yomo/pkg/pprof"

	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"

	"github.com/yomorun/yomo/internal/framework"
)

var logger = util.GetLogger("yomo::run")

// Run a server for YomoObjectPlugin
func Run(plugin plugin.YomoObjectPlugin, endpoint string) {
	logger.Infof("plugin service [%s] start... [%s]", plugin.Name(), endpoint)

	// pprof
	go pprof.Run()

	// activation service
	framework.NewServer(endpoint, plugin)
}

// RunStream run a server for YomoStreamPlugin
func RunStream(plugin plugin.YomoStreamPlugin, endpoint string) {
	logger.Infof("plugin service [%s] start... [%s]", plugin.Name(), endpoint)

	// activation service
	panic("not impl")
}

// RunDev makes test plugin connect to a demo YoMo server
func RunDev(plugin plugin.YomoObjectPlugin, endpoint string) {

	go func() {
		logger.Infof("plugin service [%s] start... [%s]", plugin.Name(), endpoint)

		// activation service
		framework.NewServer(endpoint, plugin)
	}()

	yomoEchoClient, err := util.QuicClient("161.189.140.133:11520")
	//yomoEchoClient, err := util.QuicClient("localhost:11520")
	if err != nil {
		panic(err)
	}

	yomoPluginClient, err := util.QuicClient(endpoint)
	if err != nil {
		panic(err)
	}

	go io.Copy(yomoPluginClient, yomoEchoClient) // nolint
	go io.Copy(os.Stdout, yomoPluginClient)      // nolint

	for {
		time.Sleep(time.Second)
		_, err = yomoEchoClient.Write([]byte("ping"))
		if err != nil {
			log.Fatal(err)
		}
	}

}
