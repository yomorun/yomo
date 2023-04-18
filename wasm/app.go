package main

import (
	"fmt"
	"strings"

	"github.com/yomorun/yomo/api"
)

// tinygo required main function
func main() {}

//export yomo_observe_datatag
func yomoObserveDataTag(tag uint32)

//export yomo_load_input
func yomoLoadInput(pointer *byte)

//export yomo_dump_output
func yomoDumpOutput(tag uint32, pointer *byte, length int)

//export yomo_init
func yomoInit() {
	dataTags := DataTags()
	for _, tag := range dataTags {
		yomoObserveDataTag(uint32(tag))
	}
}

//export yomo_handler
func yomoHandler(inputLength int) {
	// load input data
	input := make([]byte, inputLength)
	yomoLoadInput(&input[0])
	// handler
	ctx := api.NewContext(DataTags(), input)
	Handler(ctx)
	// tag, output := Handler(ctx)
	// dump output data
	// if output == nil {
	// 	return
	// }
	// yomoDumpOutput(uint32(tag), &output[0], len(output))
}

// Handler will handle the raw data
func Handler(ctx api.Context) error {
	data := ctx.Data()
	// cyan
	color(36, "sfn received %d bytes: %s\n", len(data), data)
	output := strings.ToUpper(string(data))
	// blue
	color(34, "sfn write %d bytes: %s\n", len(output), output)
	return ctx.Write(0x34, []byte(output))
}

func DataTags() []api.Tag {
	return []api.Tag{0x33}
}

func color(color int, format string, a ...interface{}) {
	f := fmt.Sprintf("\033[%dm%s\033[0m", color, format)
	fmt.Printf(f, a...)
}
