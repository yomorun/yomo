package main

import (
	"fmt"
	"strings"

	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/guest"
)

// tinygo required main function
func main() {
	guest.DataTags = DataTags
	guest.Handler = Handler
}

// Handler will handle the raw data
func Handler(ctx serverless.Context) {
	data := ctx.Data()
	// cyan
	color(36, "<< sfn received tag[%#x] %d bytes: %s\n", ctx.Tag(), len(data), data)
	output := strings.ToUpper(string(data))
	tag := uint32(0x34)
	// ctx.Write(tag, []byte(output))
	result := []byte(output)
	ctx.Write(tag, result)
	color(34, ">> sfn write %d bytes: %s\n", len(output), output)

	// long(1000000000)
	result2 := []byte("hello")
	ctx.Write(tag, result2)
	color(34, ">> sfn write %d bytes: %s\n", len(result2), result2)

	// long(1320000000)
	result3 := []byte("world")
	ctx.Write(tag, result3)
	color(34, ">> sfn write %d bytes: %s\n", len(result3), result3)
	// blue
	color(34, ">> sfn write all done.\n")
}

func DataTags() []uint32 {
	return []uint32{0x33}
}

func color(color int, format string, a ...interface{}) {
	f := fmt.Sprintf("\033[%dm%s\033[0m", color, format)
	fmt.Printf(f, a...)
}
