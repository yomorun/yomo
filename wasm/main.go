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
	sfn := yomo.NewStreamFunction(
		"noise",
		addr,
		yomo.WithObserveDataTags(0x33),
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
		func(hctx *serverless.HandlerContext) {
			runtime.RunHandler(hctx)
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
	WasmFuncWrite          = "yomo_write"
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
	serverlessCtx *serverless.HandlerContext
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
func (r *wazeroRuntime) RunHandler(hctx *serverless.HandlerContext) {
	log.Println("runtime.RunHandler")
	data := hctx.Data()
	r.input = data
	// reset output
	r.outputTag = 0
	r.output = nil
	//
	r.outputs = nil
	r.serverlessCtx = hctx

	// run handler
	handler := r.module.ExportedFunction(WasmFuncHandler)
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

func (r *wazeroRuntime) write(ctx context.Context, m api.Module, tag int32, pointer int32, length int32) {
	r.outputTag = uint32(tag)
	output := make([]byte, length)
	buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	}
	copy(output, buf)
	r.outputs = append(r.outputs, output)
	r.serverlessCtx.Write(uint32(tag), output)
}
