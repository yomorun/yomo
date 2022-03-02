package main

import (
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	// zipper initialize
	zipper := yomo.NewZipperWithOptions("Zipper", yomo.WithZipperAddr("localhost:9000"))
	defer zipper.Close()
	// configurate zipper workflow
	zipper.ConfigWorkflow(os.Getenv("YOMO_ZIPPER_WORKFLOW"))
	// zipper serve
	err := zipper.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
