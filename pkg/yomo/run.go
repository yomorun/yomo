package yomo

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"

	"github.com/yomorun/yomo/internal/framework"
)

// Run a server for YomoObjectPlugin
func Run(plugin plugin.YomoObjectPlugin, endpoint string) {

	log.SetPrefix(fmt.Sprintf("[%s:%v]", plugin.Name(), os.Getpid()))
	log.Printf("plugin service start... [%s]", endpoint)

	// activation service
	framework.NewServer(endpoint, plugin)
}

// RunStream run a server for YomoStreamPlugin
func RunStream(plugin plugin.YomoStreamPlugin, endpoint string) {
	log.SetPrefix(fmt.Sprintf("[%s:%v]", plugin.Name(), os.Getpid()))
	log.Printf("plugin service start... [%s]", endpoint)

	// activation service
	panic("not impl")
}

// RunDev makes test plugin connect to a demo YoMo server
func RunDev(plugin plugin.YomoObjectPlugin, endpoint string) {

	go func() {
		log.SetPrefix(fmt.Sprintf("[%s:%v]", plugin.Name(), os.Getpid()))
		log.Printf("plugin service start... [%s]", endpoint)

		// activation service
		framework.NewServer(endpoint, plugin)
	}()

	yomoEchoClient, err := util.QuicClient("161.189.140.133:11520")
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
