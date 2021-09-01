package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

func main() {
	// 初始化 zipper，指定监听本机的 udp:19000 端口
	zipper := yomo.NewZipperServer(yomo.WithName("basic-zipper"))
	defer zipper.Close()

	// 设置 sfn 的工作流：
	// CLI： 读取 workflow.yaml，
	// Cloud：从 Database 或 API 读取
	zipper.ConfigWorkflow("workflow.yaml")
	// 设置 downstream zippers：
	// CLI：通过 HTTP 请求读取（目前还未添加 auth 相关的功能，安全上会是个问题）
	// Cloud：从 Database 或 API 读取
	zipper.ConfigDownstream("can_be_read_from_http://vhq.yomo.run/dev.json")

	// 可随时添加下游级联的 Downstream Zipper
	zipper.AddDownstreamZipper(yomo.NewZipper(yomo.WithZipperEndpoint("localhost:9001")))
	// 可随时删除下游级联的 Downstream Zipper
	zipper.RemoveDownstreamZipper(yomo.NewZipper(yomo.WithZipperEndpoint("localhost:9001")))

	// 启动 Zipper 服务
	err := zipper.ListenAndServe()
	if err != nil {
		panic(err)
	}

	logger.Printf("Server has started!, pid: %d", os.Getpid())

	sgnl := make(chan struct{}, 1)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGUSR1, syscall.SIGINT)
		logger.Printf("Listening signals ...")
		for p1 := range c {
			logger.Printf("Received signal: %s", p1)
			if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
				logger.Printf("closing...%s", p1)
				close(sgnl)
			} else if p1 == syscall.SIGUSR2 {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
				fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
				fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
				fmt.Printf("\tNumGC = %v\n", m.NumGC)
			} else if p1 == syscall.SIGUSR1 {
				logger.Printf("print zipper stats(): %d", zipper.Stats())
			}
		}
		logger.Print("*** ...")
	}()

	<-sgnl
	logger.Print("Bye bye!")
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
