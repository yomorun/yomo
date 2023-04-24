//go:build tinygo || js || wasm

package tinygo

import "github.com/yomorun/yomo/api"

//export yomo_observe_datatag
func yomoObserveDataTag(tag uint32)

//export yomo_load_input
func yomoLoadInput(pointer *byte)

//export yomo_dump_output
func yomoDumpOutput(tag uint32, pointer *byte, length int)

//export yomo_init
func yomoInit() {
	dataTags := api.DataTags()
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
	ctx := api.NewContext(0x33, input)
	if ctx == nil {
		return
	}
	api.Handler(ctx)
}
