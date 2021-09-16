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
