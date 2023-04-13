package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/config"
)

func main() {
	conf, err := config.ParseWorkflowConfig("zipper_2_wf.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	zipper, err := yomo.NewZipper(
		conf.Name,
		conf.Functions,
		yomo.WithDownstreamOption(yomo.WithAuth("token", "z2")),
	)
	if err != nil {
		log.Fatalln(err)
	}
	defer zipper.Close()

	addr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

	// start zipper service
	log.Printf("Server has started!, pid: %d", os.Getpid())

	log.Fatalln(zipper.ListenAndServe(context.Background(), addr))
}
