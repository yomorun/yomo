package main

import (
	"context"
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	err := yomo.RunZipper(context.Background(), os.Getenv("YOMO_ZIPPER_WORKFLOW"), "")
	if err != nil {
		log.Fatalln(err)
	}
}
