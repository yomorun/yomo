package main

import (
	"fmt"
	"strings"

	// "github.com/yomorun/yomo/api"
	// "github.com/yomorun/yomo/api/tinygo"

	"github.com/yomorun/yomo/serverless"
)

// tinygo required main function
func main() {
	// api.NewContext = tinygo.NewContext
	serverless.NewContext = serverless.NewHandlerContext
	serverless.DataTags = DataTags
	serverless.Handler = Handler
}

// Handler will handle the raw data
// func Handler(ctx serverless.Context) {
func Handler(data []byte) {
	// func Handler(ctx *serverless.HandlerContext) {
	// data := ctx.Data()
	// cyan
	color(36, "sfn received %d bytes: %s\n", len(data), data)
	output := strings.ToUpper(string(data)) + "--ABC--"
	tag := uint32(0x34)
	// ctx.Write(tag, []byte(output))
	result := []byte(output)
	yomoWrite(tag, &result[0], len(result))
	color(34, "sfn write %d bytes: %s\n", len(output), output)

	result2 := []byte("hello")
	yomoWrite(tag, &result2[0], len(result2))
	color(34, "sfn write %d bytes: %s\n", len(result2), result2)

	result3 := []byte("world")
	yomoWrite(tag, &result3[0], len(result3))
	color(34, "sfn write %d bytes: %s\n", len(result3), result3)

	result4 := []byte("-abcdefg-")
	yomoWrite(tag, &result4[0], len(result4))
	color(34, "sfn write %d bytes: %s\n", len(result4), result4)

	result5 := []byte("-HIJKLMN-")
	yomoWrite(tag, &result5[0], len(result5))
	color(34, "sfn write %d bytes: %s\n", len(result5), result5)
	// ctx.Write(tag, []byte("-abcdefg-"))
	// ctx.Write(tag, []byte("-HIJklmn-"))
	// ctx.Write(tag, []byte("-oPqRst-"))
	// ctx.Write(tag, []byte("-uvw-"))
	// ctx.Write(tag, []byte("-XYZ-"))
	// blue
	color(34, "sfn write all done.\n")
}

func DataTags() []uint32 {
	return []uint32{0x33}
}

func color(color int, format string, a ...interface{}) {
	f := fmt.Sprintf("\033[%dm%s\033[0m", color, format)
	fmt.Printf(f, a...)
}

//export yomo_write
//go:linkname yomoWrite
func yomoWrite(tag uint32, pointer *byte, length int)
