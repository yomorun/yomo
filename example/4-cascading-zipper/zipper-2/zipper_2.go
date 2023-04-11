package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	zipper := yomo.NewZipperWithOptions(
		"zipper-2",
		"localhost:9002",
		yomo.WithAuth("token", "z2"),
	)
	defer zipper.Close()

	zipper.ConfigWorkflow("zipper_2_wf.yaml")

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
