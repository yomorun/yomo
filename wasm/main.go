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

	sfn.SetHandler(
		func(ctx serverless.Context) {
			if err := runtime.RunHandler(ctx); err != nil {
				log.Fatalln(err)
			}
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
	WasmFuncContextTag     = "yomo_context_tag"
	WasmFuncContextData    = "yomo_context_data"
)

type wazeroRuntime struct {
	wazero.Runtime
	conf   wazero.ModuleConfig
	ctx    context.Context
	module api.Module
	cache  wazero.CompilationCache

	observed      []uint32
	serverlessCtx serverless.Context
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
func (r *wazeroRuntime) RunHandler(ctx serverless.Context) error {
	r.serverlessCtx = ctx
	// run handler
	handler := r.module.ExportedFunction(WasmFuncHandler)
	if _, err := handler.Call(r.ctx); err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			return fmt.Errorf("handler.Call: %v", err)
		} else if !ok {
			return fmt.Errorf("handler.Call: %v", err)
		}
	}
	return nil
}

// Close releases all the resources related to the runtime
func (r *wazeroRuntime) Close() error {
	r.cache.Close(r.ctx)
	return r.Runtime.Close(r.ctx)
}

func (r *wazeroRuntime) observeDataTag(ctx context.Context, tag uint32) {
	r.observed = append(r.observed, uint32(tag))
}

func (r *wazeroRuntime) write(ctx context.Context, m api.Module, tag uint32, pointer uint32, length int32) uint32 {
	output := make([]byte, length)
	buf, ok := m.Memory().Read(pointer, uint32(length))
	if !ok {
		log.Printf("Memory.Read(%d, %d) out of range\n", pointer, length)
		return 1
	}
	copy(output, buf)
	if err := r.serverlessCtx.Write(uint32(tag), output); err != nil {
		return 2
	}
	return 0
}

func (r *wazeroRuntime) contextTag(ctx context.Context, m api.Module) uint32 {
	return r.serverlessCtx.Tag()
}

func (r *wazeroRuntime) contextData(ctx context.Context, m api.Module, pointer uint32, limit uint32) (dataLen uint32) {
	data := r.serverlessCtx.Data()
	dataLen = uint32(len(data))
	if dataLen > limit {
		return
	} else if dataLen == 0 {
		return
	}
	m.Memory().Write(pointer, data)
	return
}
