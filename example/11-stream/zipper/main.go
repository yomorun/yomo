package main

import (
	"context"

	"github.com/yomorun/yomo"
)

func main() {
	zipper, err := yomo.NewZipper("hello", nil, nil)
	if err != nil {
		panic(err)
	}

	err = zipper.ListenAndServe(context.Background(), "127.0.0.1:9000")
	if err != nil {
		panic(err)
	}
}
