package main

import (
	"fmt"
	"strings"

	"github.com/yomorun/yomo/api"
)

// tinygo required main function
func main() {
	// api.NewContext = tinygo.NewContext
	// serverless.NewContext = serverless.NewHandlerContext
	api.DataTags = DataTags
	api.Handler = Handler
}

// Handler will handle the raw data
/*
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
		color(34, "result4: pointer=%v, length=%v\n", &result4[0], len(result4))
		color(34, "sfn write %d bytes: %s\n", len(result4), result4)

		result5 := []byte("-HIJKLMN-")
		yomoWrite(tag, &result5[0], len(result5))
		color(34, "result5: pointer=%v, length=%v\n", &result5[0], len(result5))
		color(34, "sfn write %d bytes: %s\n", len(result5), result5)
		// ctx.Write(tag, []byte("-abcdefg-"))
		// ctx.Write(tag, []byte("-HIJklmn-"))
		// ctx.Write(tag, []byte("-oPqRst-"))
		// ctx.Write(tag, []byte("-uvw-"))
		// ctx.Write(tag, []byte("-XYZ-"))
	// blue
	color(34, "sfn write all done.\n")
}
*/

// func Handler(ctx serverless.Context, input []byte) {
func Handler(ctx *api.Context) {
	// TODO: 需要从ctx中获取
	data := ctx.Data()
	// data := input
	// cyan
	color(36, "sfn received tag[%#x] %d bytes: %s\n", ctx.Tag(), len(data), data)
	output := strings.ToUpper(string(data)) + "--ABC--"
	tag := uint32(0x34)
	// ctx.Write(tag, []byte(output))
	result := []byte(output)
	ctx.Write(tag, result)
	color(34, "sfn write %d bytes: %s\n", len(output), output)

	// long(1000000000)
	result2 := []byte("hello")
	ctx.Write(tag, result2)
	color(34, "sfn write %d bytes: %s\n", len(result2), result2)

	// long(1320000000)
	result3 := []byte("world")
	ctx.Write(tag, result3)
	color(34, "sfn write %d bytes: %s\n", len(result3), result3)
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

func long(times int) {
	fmt.Println("long running...")
	for i := 0; i < times; i++ {
	}
	fmt.Println("long running done.")
}
