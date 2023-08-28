//go:build wasmedge

// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/second-state/WasmEdge-go/wasmedge"
	wasmhttp "github.com/yomorun/yomo/cli/serverless/wasm/http"
	"github.com/yomorun/yomo/serverless"
)

type wasmEdgeRuntime struct {
	vm     *wasmedge.VM
	conf   *wasmedge.Configure
	module *wasmedge.Module

	observed      []uint32
	serverlessCtx serverless.Context
}

func newWasmEdgeRuntime() (*wasmEdgeRuntime, error) {
	conf := wasmedge.NewConfigure(wasmedge.WASI)
	vm := wasmedge.NewVMWithConfig(conf)
	wasi := vm.GetImportModule(wasmedge.WASI)
	wasi.InitWasi(
		nil,
		os.Environ(),
		[]string{".:."},
	)

	return &wasmEdgeRuntime{
		vm:   vm,
		conf: conf,
	}, nil
}

// Init loads the wasm file, and initialize the runtime environment
func (r *wasmEdgeRuntime) Init(wasmFile string) error {
	r.module = wasmedge.NewModule("env")
	// observeDataTag
	observeDataTagFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{
			wasmedge.ValType_I32,
		},
		[]wasmedge.ValType{}), r.observeDataTag, nil, 0)
	r.module.AddFunction(WasmFuncObserveDataTag, observeDataTagFunc)
	// write
	writeFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{
			wasmedge.ValType_I32,
			wasmedge.ValType_I32,
			wasmedge.ValType_I32,
		},
		[]wasmedge.ValType{wasmedge.ValType_I32},
	),
		r.write, nil, 0)
	r.module.AddFunction(WasmFuncWrite, writeFunc)
	// context tag
	contextTagFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{},
		[]wasmedge.ValType{wasmedge.ValType_I32}), r.contextTag, nil, 0)
	r.module.AddFunction(WasmFuncContextTag, contextTagFunc)
	// context data
	contextDataFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{
			wasmedge.ValType_I32,
			wasmedge.ValType_I32,
		},
		[]wasmedge.ValType{wasmedge.ValType_I32}), r.contextData, nil, 0)
	r.module.AddFunction(WasmFuncContextData, contextDataFunc)
	// context data size
	contextDataSizeFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{},
		[]wasmedge.ValType{wasmedge.ValType_I32}), r.contextDataSize, nil, 0)
	r.module.AddFunction(WasmFuncContextDataSize, contextDataSizeFunc)
	// http
	httpSendFunc := wasmedge.NewFunction(
		wasmedge.NewFunctionType(
			[]wasmedge.ValType{
				wasmedge.ValType_I32,
				wasmedge.ValType_I32,
				wasmedge.ValType_I32,
				wasmedge.ValType_I32,
			},
			[]wasmedge.ValType{wasmedge.ValType_I32},
		),
		r.httpSend, nil, 0)
	r.module.AddFunction(wasmhttp.WasmFuncHTTPSend, httpSendFunc)

	err := r.vm.RegisterModule(r.module)
	if err != nil {
		return fmt.Errorf("vm.RegisterModule: %v", err)
	}

	err = r.vm.LoadWasmFile(wasmFile)
	if err != nil {
		return fmt.Errorf("load wasm file %s: %v", wasmFile, err)
	}

	err = r.vm.Validate()
	if err != nil {
		return fmt.Errorf("vm.Validate: %v", err)
	}

	err = r.vm.Instantiate()
	if err != nil {
		return fmt.Errorf("vm.Instantiate: %v", err)
	}

	// _start
	startFunc := r.vm.GetActiveModule().FindFunction(WasmFuncStart)
	if startFunc != nil {
		_, err = r.vm.Execute(WasmFuncStart)
		if err != nil {
			return fmt.Errorf("vm.Execute %s: %v", WasmFuncStart, err)
		}
	}
	// yomo observe data tags
	observeDataTagsFunc := r.vm.GetActiveModule().FindFunction(WasmFuncObserveDataTags)
	if observeDataTagsFunc == nil {
		return fmt.Errorf("%s function not found", WasmFuncObserveDataTags)
	}
	_, err = r.vm.Execute(WasmFuncObserveDataTags)
	if err != nil {
		return fmt.Errorf("vm.Execute %s: %v", WasmFuncObserveDataTags, err)
	}

	return nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wasmEdgeRuntime) GetObserveDataTags() []uint32 {
	return r.observed
}

// RunHandler runs the wasm application (request -> response mode)
func (r *wasmEdgeRuntime) RunHandler(ctx serverless.Context) error {
	r.serverlessCtx = ctx
	// Run the handler function. Given the pointer to the input data.
	if _, err := r.vm.Execute(WasmFuncHandler); err != nil {
		return fmt.Errorf("vm.Execute %s: %v", WasmFuncHandler, err)
	}

	return nil
}

// Close releases all the resources related to the runtime
func (r *wasmEdgeRuntime) Close() error {
	r.module.Release()
	r.vm.Release()
	r.conf.Release()
	return nil
}

// RunInit runs the init function of the wasm sfn
func (r *wasmEdgeRuntime) RunInit() error {
	// init
	initFunc := r.vm.GetActiveModule().FindFunction(WasmFuncInit)
	if initFunc == nil {
		fmt.Println("init function not used")
		return nil
	}
	result, err := r.vm.Execute(WasmFuncInit)
	if err != nil {
		return fmt.Errorf("vm.Execute %s: %v", WasmFuncInit, err)
	}
	if result[0].(int32) != 0 {
		return errors.New("sfn initialization failed")
	}
	return nil
}

func (r *wasmEdgeRuntime) observeDataTag(
	_ any,
	_ *wasmedge.CallingFrame,
	params []any,
) ([]any, wasmedge.Result) {
	tag := params[0].(int32)
	r.observed = append(r.observed, uint32(tag))
	return nil, wasmedge.Result_Success
}

func (r *wasmEdgeRuntime) contextTag(
	_ any,
	callframe *wasmedge.CallingFrame,
	params []any,
) ([]any, wasmedge.Result) {
	return []any{r.serverlessCtx.Tag()}, wasmedge.Result_Success
}

func (r *wasmEdgeRuntime) contextData(
	_ any,
	callframe *wasmedge.CallingFrame,
	params []any,
) ([]any, wasmedge.Result) {
	data := r.serverlessCtx.Data()
	dataLen := int32(len(data))
	limit := params[1].(int32)
	if dataLen > limit {
		return []any{dataLen}, wasmedge.Result_Success
	} else if dataLen == 0 {
		return []any{dataLen}, wasmedge.Result_Success
	}
	pointer := params[0].(int32)
	mem := callframe.GetMemoryByIndex(0)
	if err := mem.SetData(data, uint(pointer), uint(dataLen)); err != nil {
		return []any{0}, wasmedge.Result_Fail
	}
	return []any{dataLen}, wasmedge.Result_Success
}

func (r *wasmEdgeRuntime) contextDataSize(
	_ any,
	callframe *wasmedge.CallingFrame,
	params []any,
) ([]any, wasmedge.Result) {
	dataLen := len(r.serverlessCtx.Data())
	return []any{dataLen}, wasmedge.Result_Success
}

func (r *wasmEdgeRuntime) write(
	_ any,
	callframe *wasmedge.CallingFrame,
	params []any,
) ([]any, wasmedge.Result) {
	tag := params[0].(int32)
	pointer := params[1].(int32)
	length := params[2].(int32)
	mem := callframe.GetMemoryByIndex(0)
	output, err := mem.GetData(uint(pointer), uint(length))
	if err != nil {
		return []any{1}, wasmedge.Result_Fail
	}
	buf := make([]byte, length)
	copy(buf, output)
	if err := r.serverlessCtx.Write(uint32(tag), buf); err != nil {
		return []any{2}, wasmedge.Result_Fail
	}
	return []any{0}, wasmedge.Result_Success
}

// httpSend sends http request
func (r *wasmEdgeRuntime) httpSend(
	_ any,
	callframe *wasmedge.CallingFrame,
	params []any,
) ([]any, wasmedge.Result) {
	reqPtr := params[0].(int32)
	reqSize := params[1].(int32)
	mem := callframe.GetMemoryByIndex(0)
	reqBuf, err := mem.GetData(uint(reqPtr), uint(reqSize))
	if err != nil {
		return []any{1}, wasmedge.Result_Fail
	}
	respBuf, err := wasmhttp.Do(reqBuf)
	if err != nil {
		log.Printf("[HTTP] Send: %s\n", err)
		return []any{2}, wasmedge.Result_Fail
	}
	respPtr := params[2].(int32)
	respSize := params[3].(int32)
	// write response
	allocFn := r.vm.GetActiveModule().FindFunction("yomo_alloc")
	if allocFn == nil {
		log.Printf("[HTTP] Send: yomo_alloc not found\n")
		return []any{3}, wasmedge.Result_Fail
	}
	// alloc memory
	dataLen := len(respBuf)
	allocResult, err := r.vm.Execute("yomo_alloc", int32(dataLen))
	if err != nil {
		log.Printf("[HTTP] Send: yomo_alloc error: %s\n", err)
		return []any{4}, wasmedge.Result_Fail
	}
	allocPtr := int32(allocResult[0].(int32))
	// set response pointer
	allocPtrBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(allocPtrBuf, uint32(allocPtr))
	if err := mem.SetData(allocPtrBuf, uint(respPtr), 4); err != nil {
		log.Printf("[HTTP] Send: set response pointer error: %s\n", err)
		return []any{5}, wasmedge.Result_Fail
	}
	// set response size
	allocSizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(allocSizeBuf, uint32(dataLen))
	if err := mem.SetData(allocSizeBuf, uint(respSize), 4); err != nil {
		log.Printf("[HTTP] Send: set response size error: %s\n", err)
		return []any{5}, wasmedge.Result_Fail
	}
	// set response data
	if err := mem.SetData(respBuf, uint(allocPtr), uint(dataLen)); err != nil {
		log.Printf("[HTTP] Send: set response data error: %s\n", err)
		return []any{5}, wasmedge.Result_Fail
	}
	return []any{0}, wasmedge.Result_Success
}
