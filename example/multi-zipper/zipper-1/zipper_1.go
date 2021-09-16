package main

import (
	"log"
	"os"
	"runtime"

	"github.com/yomorun/yomo"
)

func main() {
	zipper, err := yomo.NewZipper("workflow.yaml")
	if err != nil {
		panic(err)
	}
	defer zipper.Close()

	// add Downstream Zipper
	zipper.AddDownstreamZipper(yomo.NewDownstreamZipper("z1", yomo.WithZipperAddr("localhost:9002")))
	// zipper.RemoveDownstreamZipper(yomo.NewZipper("z1", yosmo.WithZipperAddr("localhost:9001")))

	// start zipper service
	log.Printf("Server has started!, pid: %d", os.Getpid())
	go func() {
		err = zipper.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
	runtime.Goexit()
}
