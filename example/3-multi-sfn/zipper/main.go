package main

import (
	"context"
	"log"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/trace"
)

func main() {
	// trace
	tp, shutdown, err := trace.NewTracerProviderWithJaeger("zipper")
	if err == nil {
		log.Println("[zipper] 🛰 trace enabled")
	}
	defer shutdown(context.Background())
	// zipper
	conf, err := config.ParseConfigFile("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	options := []yomo.ZipperOption{
		yomo.WithZipperTracerProvider(tp),
	}

	zipper, err := yomo.NewZipper(conf.Name, conf.Functions, conf.Downstreams, options...)
	if err != nil {
		log.Fatal(err)
	}
	// zipper.Logger().Info("using config file", "file_path", configPath)

	zipper.ListenAndServe(context.Background(), "0.0.0.0:9000")
}
