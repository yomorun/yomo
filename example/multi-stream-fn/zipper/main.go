package main

import (
	"github.com/yomorun/yomo"
)

func main() {
	// zipper initialize
	zipper := yomo.NewZipperWithOptions("Zipper", yomo.WithZipperAddr("localhost:9000"))
	defer zipper.Close()
	// configurate zipper workflow
	zipper.ConfigWorkflow("workflow.yaml")
	// zipper serve
	err := zipper.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
