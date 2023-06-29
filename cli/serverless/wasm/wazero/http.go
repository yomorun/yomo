package wazero

import (
	"context"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	wasmhttp "github.com/yomorun/yomo/cli/serverless/wasm/http"
)

func ExportHTTPHostFuncs(builder wazero.HostModuleBuilder) {
	builder.
		// get
		NewFunctionBuilder().
		WithGoModuleFunction(
			api.GoModuleFunc(Send),
			[]api.ValueType{
				api.ValueTypeI32, // reqPtr
				api.ValueTypeI32, // reqSize
				api.ValueTypeI32, // respPtr
				api.ValueTypeI32, // respSize
			},
			[]api.ValueType{api.ValueTypeI32}, // ret
		).
		Export(wasmhttp.WasmFuncHTTPSend)
}

// Send sends a HTTP request and returns the response
func Send(ctx context.Context, m api.Module, stack []uint64) {
	// request
	reqPtr := uint32(stack[0])
	reqSize := uint32(stack[1])
	reqBuf, err := readBuffer(ctx, m, reqPtr, reqSize)
	if err != nil {
		log.Printf("[HTTP] Send: get request error: %s\n", err)
		stack[0] = 1
		return
	}
	// response
	respBuf, err := wasmhttp.Do(reqBuf)
	if err != nil {
		log.Printf("[HTTP] Send: %s\n", err)
		stack[0] = 2
		return
	}
	respPtr := uint32(stack[2])
	respSize := uint32(stack[3])
	if err := allocateBuffer(ctx, m, respPtr, respSize, respBuf); err != nil {
		log.Printf("[HTTP] Send: write response error: %s\n", err)
		stack[0] = 4
		return
	}
	// return
	stack[0] = 0
}
