package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	zipper, err := yomo.NewZipper("workflow.yaml")
	if err != nil {
		panic(err)
	}
	defer zipper.Close()

	// add Downstream Zipper
	// zipper.AddDownstreamZipper(yomo.NewZipper("z1", yomo.WithZipperAddr("localhost:9001")))
	// zipper.RemoveDownstreamZipper(yomo.NewZipper("z1", yosmo.WithZipperAddr("localhost:9001")))

	// start zipper service
	log.Printf("Server has started!, pid: %d", os.Getpid())
	err = zipper.ListenAndServe()
	if err != nil {
		panic(err)
	}
	log.Print("Bye bye!")
}
