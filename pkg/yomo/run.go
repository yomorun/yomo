package yomo

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	txtkv "github.com/10cella/yomo-txtkv-codec"

	"github.com/yomorun/yomo/pkg/plugin"
	"github.com/yomorun/yomo/pkg/util"

	"github.com/yomorun/yomo/internal/framework"
)

// Run a server for YomoObjectPlugin
func Run(plugin plugin.YomoObjectPlugin, endpoint string) {

	log.SetPrefix(fmt.Sprintf("[%s:%v]", plugin.Name(), os.Getpid()))
	log.Printf("plugin servie start... [%s]", endpoint)

	// binding plugin
	pluginStream := framework.NewObjectPlugin(plugin)

	// decoding
	deStream1 := txtkv.NewObjectDecoder(plugin.Observed())

	//过滤
	deStream2 := txtkv.NewFilterDecoder(plugin.Observed())

	// encoding
	enStream := txtkv.NewObjectEncoder(plugin.Observed())

	deStream := io.MultiWriter(deStream1.Writer, deStream2.Writer)

	go func() { io.CopyN(pluginStream.Writer, deStream1.Reader, 1024) }() // nolint
	go func() { io.CopyN(enStream.Writer, pluginStream.Reader, 1024) }()  // nolint
	go func() { io.CopyN(enStream.Writer, deStream2.Reader, 1024) }()     // nolint

	// activation service
	framework.NewServer(endpoint, deStream, enStream.Reader)
}

// Run a server for YomoStreamPlugin
func RunStream(plugin plugin.YomoStreamPlugin, endpoint string) {
	log.SetPrefix(fmt.Sprintf("[%s:%v]", plugin.Name(), os.Getpid()))
	log.Printf("plugin servie start... [%s]", endpoint)

	// binding plugin
	pluginStream := framework.NewStreamPlugin(plugin)

	// decoding
	deStream1 := txtkv.NewStreamDecoder(plugin.Observed())

	//过滤
	deStream2 := txtkv.NewFilterDecoder(plugin.Observed())

	// encoding
	enStream := txtkv.NewStreamEncoder(plugin.Observed())

	deStream := io.MultiWriter(deStream1.Writer, deStream2.Writer)

	// activation service
	framework.NewServer(endpoint, deStream, enStream.Reader)

	go func() { io.CopyN(pluginStream.Writer, deStream1.Reader, 1024) }() // nolint
	go func() { io.CopyN(enStream.Writer, pluginStream.Reader, 1024) }()  // nolint
	go func() { io.CopyN(enStream.Writer, deStream2.Reader, 1024) }()     // nolint
}

func RunDev(plugin plugin.YomoObjectPlugin, endpoint string) {

	go func() {
		log.SetPrefix(fmt.Sprintf("[%s:%v]", plugin.Name(), os.Getpid()))
		log.Printf("plugin servie start... [%s]", endpoint)

		// binding plugin
		pluginStream := framework.NewObjectPlugin(plugin)

		// decoding
		deStream1 := txtkv.NewObjectDecoder(plugin.Observed())

		//过滤
		deStream2 := txtkv.NewFilterDecoder(plugin.Observed())

		// encoding
		enStream := txtkv.NewObjectEncoder(plugin.Observed())

		deStream := io.MultiWriter(deStream1.Writer, deStream2.Writer)

		go func() { io.CopyN(pluginStream.Writer, deStream1.Reader, 1024) }() // nolint
		go func() { io.CopyN(enStream.Writer, pluginStream.Reader, 1024) }()  // nolint
		go func() { io.CopyN(enStream.Writer, deStream2.Reader, 1024) }()     // nolint

		// activation service
		framework.NewServer(endpoint, deStream, enStream.Reader)
	}()

	yomoEchoClient, _ := util.QuicClient("echo.cella.fun:11521")
	yomoPluginClient, _ := util.QuicClient(endpoint)

	go io.Copy(yomoPluginClient, yomoEchoClient)
	go io.Copy(os.Stdout, yomoPluginClient)

	for {
		time.Sleep(time.Second)
		yomoEchoClient.Write([]byte("ping"))
	}

}
