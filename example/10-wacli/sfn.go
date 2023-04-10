//go:generate tinygo build -o sfn.wasm -no-debug -target wasi

package main

import (
	"fmt"
	"strings"
)

func main() {}

//export yomo_observe_datatag
func yomoObserveDataTag(tag uint32)

//export yomo_load_input
func yomoLoadInput(pointer *byte)

//export yomo_dump_output
func yomoDumpOutput(tag uint32, pointer *byte, length int)

//export yomo_init
func yomoInit() {
	yomoObserveDataTag(0x33)
}

//export yomo_handler
func yomoHandler(inputLength int) {
	fmt.Printf("wasm go sfn received %d bytes\n", inputLength)

	// load input data
	input := make([]byte, inputLength)
	yomoLoadInput(&input[0])

	// process app data
	output := strings.ToUpper(string(input))

	// dump output data
	yomoDumpOutput(0x34, &[]byte(output)[0], len(output))
}
