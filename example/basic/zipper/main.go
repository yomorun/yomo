package main

import (
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

func main() {
	zipper := yomo.NewZipperWithOptions("basic-zipper", yomo.WithZipperAddr("localhost:9000"))
	defer zipper.Close()

	zipper.ConfigWorkflow("workflow.yaml")

	// start zipper service
	go func(zipper yomo.Zipper) {
		err := zipper.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}(zipper)

	logger.Printf("Server has started!, pid: %d", os.Getpid())
	for {
		select {}
	}
}
