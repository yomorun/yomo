package main

import (
	"flag"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/logger"
)

const (
	DefaultAddr           = "localhost:9000"
	DefaultDownstreamAddr = ""
)

func main() {
	flag.Parse()
	zipper := yomo.NewZipperWithOptions(
		"basic-zipper",
		yomo.WithZipperAddr(env("YOMO_ADDR", DefaultAddr)),
	)
	defer zipper.Close()

	err := zipper.ConfigWorkflow("workflow.yaml")
	if err != nil {
		panic(err)
	}
	// add downstream
	downAddr := env("YOMO_DOWNSTREAM_ADDR")
	if downAddr != "" {
		zipper.AddDownstreamZipper(yomo.NewDownstreamZipper("downstream-zipper", yomo.WithZipperAddr(downAddr)))
	}

	// start zipper service
	go func(zipper yomo.Zipper) {
		err := zipper.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}(zipper)

	logger.Printf("Server has started!, pid: %d", os.Getpid())
	select {}
}

func env(key string, defaults ...string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return ""
}
