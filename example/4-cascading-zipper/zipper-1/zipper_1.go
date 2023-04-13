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
	conf, err := config.ParseWorkflowConfig("zipper_1_wf.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	zipper, err := yomo.NewZipper(
		conf.Name,
		conf.Functions,
		yomo.WithDownstreamOption(yomo.WithAuth("token", "z1")),
		yomo.WithMeshConfigProvider(yomo.DefaultMeshConfigProvider(config.MeshZipper{
			Name:       "zipper-2",
			Host:       "localhost",
			Port:       9002,
			Credential: "token:z2",
		})),
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
