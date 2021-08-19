package main

import (
	"fmt"

	server "github.com/yomorun/yomo/zipper"
)

func main() {
	conf, err := server.ParseConfig("./workflow.yaml")
	if err != nil {
		panic(err)
	}
	rt := server.New(conf, server.WithMeshConfURL(""))
	err = rt.Serve(fmt.Sprintf("%s:%d", conf.Host, conf.Port))
	if err != nil {
		panic(err)
	}
}
