package yomo

import (
	"fmt"
	"io"
	"log"
	"time"

	y3 "github.com/yomorun/yomo-codec-golang"
	"github.com/yomorun/yomo-codec-golang/pkg/codes"
	"github.com/yomorun/yomo-codec-golang/pkg/codes/packetstructure"

	"github.com/yomorun/yomo-codec-golang/pkg/packetutils"

	"github.com/yomorun/yomo/configs"

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
	RunDevWith(plugin, endpoint, OutputPacketPrinter)
}

type OutputFormatter int32

const (
	OutputHexString       OutputFormatter = 0
	OutputPacketPrinter   OutputFormatter = 1
	OutputEchoData        OutputFormatter = 2
	OutputThermometerData OutputFormatter = 3
)

// RunDev makes test plugin connect to a demo YoMo server. OutputFormatter: OutputHexString/OutputPacketPrinter/OutputEchoData
func RunDevWith(plugin plugin.YomoObjectPlugin, endpoint string, formatter OutputFormatter) {
	go func() {
		logger.Infof("plugin service [%s] start... [%s]", plugin.Name(), endpoint)

		// activation service
		framework.NewServer(endpoint, plugin)
	}()

	yomoEchoClient, err := util.QuicClient(configs.DefaultEchoConf.EchoServerAddr)
	//yomoEchoClient, err := util.QuicClient("localhost:11520")
	if err != nil {
		panic(err)
	}

	yomoPluginClient, err := util.QuicClient(endpoint)
	if err != nil {
		panic(err)
	}

	go io.Copy(yomoPluginClient, yomoEchoClient) // nolint

	// select formatter
	var w io.Writer
	switch formatter {
	case OutputHexString:
		w = &hexStringFormatter{}
	case OutputPacketPrinter:
		w = &packetPrinterFormatter{}
	case OutputEchoData:
		w = &echoDataFormatter{}
	case OutputThermometerData:
		w = &thermometerDataFormatter{}
	default:
		w = &packetPrinterFormatter{}
	}
	go util.CopyTo(w, yomoPluginClient) // nolint

	for {
		time.Sleep(time.Second)
		_, err = yomoEchoClient.Write([]byte("ping"))
		if err != nil {
			log.Fatal(err)
		}
	}
}

// hexStringFormatter
type hexStringFormatter struct {
	io.Writer
}

func (w *hexStringFormatter) Write(b []byte) (int, error) {
	fmt.Printf("%v:\t %s\n", time.Now().Format("2006-01-02 15:04:05"), packetutils.FormatBytes(b)) // debug:
	return 0, nil
}

// packetPrinterFormatter
type packetPrinterFormatter struct {
	io.Writer
}

func (w *packetPrinterFormatter) Write(b []byte) (int, error) {
	res, _, _ := y3.DecodeNodePacket(b)
	fmt.Printf("%v:\t", time.Now().Format("2006-01-02 15:04:05")) // debug:
	packetutils.PrintNodePacket(res)
	fmt.Println()
	return 0, nil
}

// echoDataFormatter
type echoDataFormatter struct {
	io.Writer
}

func (w *echoDataFormatter) Write(b []byte) (int, error) {
	var mold = echoData{}
	res, _, _ := y3.DecodeNodePacket(b)
	_ = packetstructure.Decode(res, &mold)
	fmt.Printf("%v:\t %v\n", time.Now().Format("2006-01-02 15:04:05"), mold) // debug:
	return 0, nil
}

type echoData struct {
	Id   int32  `yomo:"0x10"`
	Name string `yomo:"0x11"`
	Test test   `yomo:"0x20"`
}

type test struct {
	Tag []string `yomo:"0x13"`
}

// thermometerDataFormatter
type thermometerDataFormatter struct {
	io.Writer
}

func (w *thermometerDataFormatter) Write(b []byte) (int, error) {
	var mold = []thermometerData{}

	proto := codes.NewProtoCodec("0x20")
	proto.UnmarshalStruct(b, &mold)
	fmt.Printf("%v:\t %v\n", time.Now().Format("2006-01-02 15:04:05"), mold) // debug:
	return 0, nil
}

type thermometerData struct {
	Id          string  `yomo:"0x10"`
	Temperature float32 `yomo:"0x11"`
	Humidity    float32 `yomo:"0x12"`
	Stored      bool    `yomo:"0x13"`
}
