package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/yomorun/yomo"
)

func main() {
	zipper := yomo.NewZipperServer("basic-zipper", yomo.WithZipperListenAddr("localhost:9000"))
	defer zipper.Close()

	zipper.ConfigWorkflow("workflow.yaml")
	// zipper.ConfigDownstream("can_be_read_from_http://vhq.yomo.run/dev.json")

	// add Downstream Zipper
	zipper.AddDownstreamZipper(yomo.NewZipper("z1", yomo.WithZipperAddr("localhost:9001")))
	// zipper.RemoveDownstreamZipper(yomo.NewZipper("z1", yosmo.WithZipperAddr("localhost:9001")))

	// start zipper service
	go func(zipper yomo.Zipper) {
		err := zipper.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}(zipper)

	log.Printf("Server has started!, pid: %d", os.Getpid())

	sgnl := make(chan struct{})

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGUSR1, syscall.SIGINT)
		log.Printf("Listening signals ...")
		for p1 := range c {
			log.Printf("Received signal: %s", p1)
			if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
				log.Printf("clsosing...%s", p1)
				close(sgnl)
			} else if p1 == syscall.SIGUSR2 {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
				fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
				fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
				fmt.Printf("\tNumGC = %v\n", m.NumGC)
			} else if p1 == syscall.SIGUSR1 {
				log.Printf("print zipper stats(): %d", zipper.Stats())
			}
		}
		log.Print("*** ...")
	}()

	<-sgnl
	log.Print("Bye bye!")
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
