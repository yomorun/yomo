//go:generate tinygo build -o sfn.wasm -no-debug -target wasi

package main

import (
	"fmt"
	"strings"
	// "github.com/yomorun/yomo/core/frame"
)

type Tag uint32

func main() {}

//export yomo_observe_datatag
func yomoObserveDataTag(tag uint32)

//export yomo_load_input
func yomoLoadInput(pointer *byte)

//export yomo_dump_output
func yomoDumpOutput(tag uint32, pointer *byte, length int)

//export yomo_init
func yomoInit() {
	// yomoObserveDataTag(0x33)
	dataTags := DataTags()
	for _, tag := range dataTags {
		yomoObserveDataTag(uint32(tag))
	}
}

//export yomo_handler
func yomoHandler(inputLength int) {
	fmt.Printf("wasm go sfn received %d bytes\n", inputLength)

	// load input data
	input := make([]byte, inputLength)
	yomoLoadInput(&input[0])

	// process app data
	// output := strings.ToUpper(string(input))

	// dump output data
	// yomoDumpOutput(0x34, &[]byte(output)[0], len(output))

	tag, output := Handler(input)
	yomoDumpOutput(uint32(tag), &output[0], len(output))
}

// func handler(data []byte) (frame.Tag, []byte) {
// 	output := strings.ToUpper(string(data))
// 	res := []byte(output)
// 	tag := frame.Tag(0x34)
// 	return tag, res
// }

//-------------------------------------------------------------------------------------------
// Native Handler
//-------------------------------------------------------------------------------------------

func Handler(data []byte) (Tag, []byte) {
	output := strings.ToUpper(string(data))
	res := []byte(output)
	// tag := frame.Tag(0x34)
	tag := Tag(0x34)
	return tag, res
}

// func DataTags() []frame.Tag {
func DataTags() []Tag {
	return []Tag{0x33}
}
