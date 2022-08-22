package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	zipper, err := yomo.NewZipper("zipper_1_wf.yaml")
	if err != nil {
		panic(err)
	}
	zipper.InitOptions(yomo.WithAuth("token", "z1"))
	defer zipper.Close()

	// add Downstream Zipper
	zipper.AddDownstreamZipper(yomo.NewDownstreamZipper(
		"zipper-2",
		yomo.WithZipperAddr("localhost:9002"),
		yomo.WithCredential("token:z2"),
	))

	// start zipper service
	log.Printf("Server has started!, pid: %d", os.Getpid())
	go func() {
		err := zipper.ListenAndServe()
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
	}()
	select {}
}
