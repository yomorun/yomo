package main

import (
	"context"
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	err := yomo.RunZipper(context.Background(), os.Getenv("YOMO_ZIPPER_WORKFLOW"), os.Getenv("YOMO_ZIPPER_MESH_URL"))
	if err != nil {
		log.Fatalln(err)
	}
}
