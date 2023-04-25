package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

// =======================================================================
// main
// =======================================================================
func main() {
	runtime, err := newWazeroRuntime()
	if err != nil {
		log.Fatalln(err)
	}

	err = runtime.Init("./sfn.wasm")
	if err != nil {
		log.Fatalln(err)
	}
	addr := "127.0.0.1:9000"
	observed := runtime.GetObserveDataTags()
	sfn := yomo.NewStreamFunction(
		"noise",
		addr,
		yomo.WithObserveDataTags(observed...),
	)

	// sfn.SetHandler(
	// 	func(req []byte) (uint32, []byte) {
	// 		tag, res, err := s.runtime.RunHandler(req)
	// 		if err != nil {
	// 			ch <- err
	// 		}

	// 		return tag, res
	// 	},
	// )
	sfn.SetHandler(
		func(ctx *serverless.Context) {
			runtime.RunHandler(ctx)
			// req := hctx.Data()
			// tag, res, err := s.runtime.RunHandler(req)
			// if err != nil {
			// 	ch <- err
			// }
			// s.runtime.RunHandler(req)
			// tag, outputs := s.runtime.Outputs()
			// outputs = append(outputs, []byte("-ABC-"))
			// outputs = append(outputs, []byte("-def-"))
			// outputs = append(outputs, []byte("-GGG-"))
			// for _, output := range outputs {
			// 	fmt.Printf("wasm serverless handler: got tag[%#x], output_tag=%#x, result=%s\n", hctx.Tag(), tag, output)
			// 	if err := hctx.Write(tag, output); err != nil {
			// 		fmt.Printf("wasm serverless handler: write error: %v\n", err)
			// 		return
			// 	}
			// }
		},
	)

	sfn.SetErrorHandler(
		func(err error) {
			log.Printf("[wasm][%s] error handler: %T %v\n", addr, err, err)
		},
	)

	err = sfn.Connect()
	if err != nil {
		log.Fatalln(err)
	}
	defer sfn.Close()
	defer runtime.Close()

	select {}
}

// =======================================================================
// wasm runtime
// =======================================================================
const (
	WasmFuncInit           = "yomo_init"
	WasmFuncObserveDataTag = "yomo_observe_datatag"
	WasmFuncLoadInput      = "yomo_load_input"
	WasmFuncDumpOutput     = "yomo_dump_output"
	WasmFuncHandler        = "yomo_handler"
	//
	WasmFuncWrite       = "yomo_write"
	WasmFuncContextTag  = "yomo_context_tag"
	WasmFuncContextData = "yomo_context_data"
)

type wazeroRuntime struct {
	wazero.Runtime
	conf   wazero.ModuleConfig
	ctx    context.Context
	module api.Module
	cache  wazero.CompilationCache

	observed  []uint32
	input     []byte
	outputTag uint32
	output    []byte
	//
	outputs       [][]byte
	serverlessCtx *serverless.Context
	inputPtr      uint32
	inputSize     uint32
}

func newWazeroRuntime() (*wazeroRuntime, error) {
	ctx := context.Background()
	cache := wazero.NewCompilationCache()
	runConfig := wazero.NewRuntimeConfig().
		WithCompilationCache(cache)
	r := wazero.NewRuntimeWithConfig(ctx, runConfig)
	// Instantiate WASI, which implements host functions needed for TinyGo to implement `panic`.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().
		// WithStartFunctions().
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	return &wazeroRuntime{
		Runtime: r,
		conf:    config,
		ctx:     ctx,
		cache:   cache,
		outputs: make([][]byte, 0),
	}, nil
}

// Init loads the wasm file, and initialize the runtime environment
func (r *wazeroRuntime) Init(wasmFile string) error {
	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		return fmt.Errorf("read wasm file %s: %v", wasmBytes, err)
	}
	builder := r.NewHostModuleBuilder("env")
	_, err = builder.NewFunctionBuilder().
		// observeDataTag
		WithFunc(r.observeDataTag).
		Export(WasmFuncObserveDataTag).
		// loadInput
		NewFunctionBuilder().
		WithFunc(r.loadInput).
		Export(WasmFuncLoadInput).
		// dumpOutput
		NewFunctionBuilder().
		WithFunc(r.dumpOutput).
		Export(WasmFuncDumpOutput).
		// write
		NewFunctionBuilder().
		WithFunc(r.write).
		Export(WasmFuncWrite).
		// context tag
		NewFunctionBuilder().
		WithFunc(r.contextTag).
		Export(WasmFuncContextTag).
		// context data
		NewFunctionBuilder().
		WithFunc(r.contextData).
		Export(WasmFuncContextData).
		// Instantiate
		Instantiate(r.ctx)
	if err != nil {
		return fmt.Errorf("wazero.HostFunc: %v", err)
	}

	module, err := r.InstantiateWithConfig(r.ctx, wasmBytes, r.conf)
	if err != nil {
		return fmt.Errorf("wazero.Module: %v", err)
	}
	r.module = module

	init := module.ExportedFunction(WasmFuncInit)

	if _, err := init.Call(r.ctx); err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			return fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
		} else if !ok {
			return fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
		}
	}

	return nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wazeroRuntime) GetObserveDataTags() []uint32 {
	return r.observed
}

// RunHandler runs the wasm application (request -> response mode)
// func (r *wazeroRuntime) RunHandler(data []byte) (uint32, []byte, error) {
// 	r.input = data
// 	// reset output
// 	r.outputTag = 0
// 	r.output = nil
// 	r.outputs = nil

// 	// run handler
// 	handler := r.module.ExportedFunction(WasmFuncHandler)
// 	if _, err := handler.Call(r.ctx, uint64(len(data))); err != nil {
// 		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
// 			return 0, nil, fmt.Errorf("handler.Call: %v", err)
// 		} else if !ok {
// 			return 0, nil, fmt.Errorf("handler.Call: %v", err)
// 		}
// 	}

//		return r.outputTag, r.output, nil
//	}
func (r *wazeroRuntime) RunHandler(hctx *serverless.Context) {
	log.Println("runtime.RunHandler")
	data := hctx.Data()
	r.input = data
	// reset output
	r.outputTag = 0
	r.output = nil
	//
	r.outputs = nil
	r.serverlessCtx = hctx
	// TODO: 需要调整 tinygo 内存分配函数的依赖
	// 设置 data 内存
	malloc := r.module.ExportedFunction("malloc")
	free := r.module.ExportedFunction("free")
	// Let's use the argument to this main function in Wasm.
	dataSize := uint64(len(data))

	// Instead of an arbitrary memory offset, use TinyGo's allocator. Notice
	// there is nothing string-specific in this allocation function. The same
	// function could be used to pass binary serialized data to Wasm.
	results, err := malloc.Call(r.ctx, dataSize)
	if err != nil {
		log.Panicln(err)
	}
	dataPtr := results[0]
	// This pointer is managed by TinyGo, but TinyGo is unaware of external usage.
	// So, we have to free it when finished
	defer free.Call(r.ctx, dataPtr)

	// The pointer is a linear memory offset, which is where we write the data
	if !r.module.Memory().Write(uint32(dataPtr), data) {
		log.Panicf("Memory.Write(%d, %d) out of range of memory size %d",
			dataPtr, dataSize, r.module.Memory().Size())
	}
	// 保存 input 内存地址
	r.inputPtr = uint32(dataPtr)
	r.inputSize = uint32(dataSize)

	// run handler
	handler := r.module.ExportedFunction(WasmFuncHandler)
	// TODO: 需要在内部构建ctx，无法通过参数传递
	if _, err := handler.Call(r.ctx, uint64(len(data))); err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			log.Fatalf("handler.Call 0: %v\n", err)
		} else if !ok {
			log.Fatalf("handler.Call 1: %v\n", err)
		}
	}

	// return r.outputTag, r.output, nil
}

func (r *wazeroRuntime) Outputs() (uint32, [][]byte) {
	return r.outputTag, r.outputs
}

// Close releases all the resources related to the runtime
func (r *wazeroRuntime) Close() error {
	r.cache.Close(r.ctx)
	return r.Runtime.Close(r.ctx)
}

func (r *wazeroRuntime) observeDataTag(ctx context.Context, tag int32) {
	r.observed = append(r.observed, uint32(tag))
}

func (r *wazeroRuntime) loadInput(ctx context.Context, m api.Module, pointer int32) {
	if !m.Memory().Write(uint32(pointer), r.input) {
		log.Panicf("Memory.Write(%d, %d) out of range of memory size %d",
			pointer, len(r.input), m.Memory().Size())
	}
}

func (r *wazeroRuntime) dumpOutput(ctx context.Context, m api.Module, tag int32, pointer int32, length int32) {
	r.outputTag = uint32(tag)
	r.output = make([]byte, length)
	buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	}
	copy(r.output, buf)
}

// ============================
func (r *wazeroRuntime) write(ctx context.Context, m api.Module, tag int32, pointer int32, length int32) {
	// r.outputTag = uint32(tag)
	output := make([]byte, length)
	buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	}
	copy(output, buf)
	// r.outputs = append(r.outputs, output)
	// r.serverlessCtx.Write(uint32(tag), output)
	// metaFrame := r.serverlessCtx.GetMetaFrame()
	// dataFrame := frame.NewDataFrame()
	// // reuse transactionID
	// dataFrame.SetTransactionID(metaFrame.TransactionID())
	// // reuse sourceID
	// dataFrame.SetSourceID(metaFrame.SourceID())
	// dataFrame.SetCarriage(uint32(tag), output)
	// // TODO: 返回值
	// r.serverlessCtx.WriteFrame(dataFrame)
	r.serverlessCtx.Write(uint32(tag), output)
}

func (r *wazeroRuntime) contextTag(ctx context.Context, m api.Module) uint32 {
	return r.serverlessCtx.Tag()
}

func (r *wazeroRuntime) contextData(ctx context.Context, m api.Module) uint64 {
	// buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	// if !ok {
	// 	log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	// }
	// resultPtr := buf[0]
	// resultLen := len(buf)
	// result := PackUint32(uint32(resultPtr), uint32(resultLen))
	// return result
	// resultPtr := uintptr(unsafe.Pointer(&r.input[0]))
	// resultLen := len(r.input)
	// result := PackUint32(uint32(resultPtr), uint32(resultLen))
	// return result
	// pointer:=&r.input[0]
	// buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	// if !ok {
	// 	log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	// }
	resultPtr := r.inputPtr
	resultSize := r.inputSize
	result := PackUint32(uint32(resultPtr), uint32(resultSize))
	return result
}

func PackUint32(offset uint32, length uint32) uint64 {
	return uint64(offset)<<32 | uint64(length)
}

func UnpackUint32(packed uint64) (uint32, uint32) {
	return uint32(packed >> 32), uint32(packed)
}
