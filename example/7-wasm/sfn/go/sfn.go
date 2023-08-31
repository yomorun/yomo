package main

import (
	"fmt"
	"strings"

	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/guest"
)

func main() {
	guest.DataTags = DataTags
	guest.Handler = Handler
	guest.Init = Init
}

// Init will initialize the stream function
func Init() error {
	fmt.Println("wasm go sfn init")
	return nil
}

func Handler(ctx serverless.Context) {
	// load input data
	tag := ctx.Tag()
	input := ctx.Data()
	fmt.Printf("wasm go sfn received %d bytes with tag[%#x]\n", len(input), tag)

	// process app data
	output := strings.ToUpper(string(input))

	// dump output data
	ctx.Write(0x34, []byte(output))
}

func DataTags() []uint32 {
	return []uint32{0x33}
}
