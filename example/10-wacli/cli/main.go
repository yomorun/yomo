package main

import (
	"log"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
)

func main() {
	cllose, err := run("../sfn/sfn.wasm")
	if err != nil {
		log.Fatal(err)
	}
	defer cllose()
	select {}
}

func run(wasmFile string) (func() error, error) {
	runtime, err := newWazeroRuntime()
	if err != nil {
		log.Printf("newWazeroRuntime error: %v\n", err)
		return nil, err
	}
	defer runtime.Close()

	err = runtime.Init(wasmFile)
	if err != nil {
		log.Printf("runtime.Init error: %v\n", err)
		return nil, err
	}

	name := "upper"
	addr := "localhost:9000"
	tags := runtime.GetObserveDataTags()
	// tags = []frame.Tag{0x33}
	sfn := yomo.NewStreamFunction(
		name,
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(tags...),
		// yomo.WithCredential(s.credential),
	)

	sfn.SetHandler(
		func(req []byte) (frame.Tag, []byte) {
			tag, res, err := runtime.RunHandler(req)
			if err != nil {
				log.Printf("runtime.RunHandler error: %v\n", err)
				return 0, nil
			}
			// output := strings.ToUpper(string(req))
			// res := []byte(output)
			// tag := frame.Tag(0x34)
			return tag, res
		},
	)

	sfn.SetErrorHandler(
		func(err error) {
			log.Printf("[wasm][%s] error handler: %T %v\n", addr, err, err)
		},
	)

	err = sfn.Connect()
	if err != nil {
		log.Printf("sfn.Connect error: %v\n", err)
		return nil, err
	}
	return sfn.Close, nil
}
