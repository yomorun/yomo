// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"github.com/yomorun/yomo/serverless"
)

const i32 = api.ValueTypeI32

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
		WithSysWalltime().
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
	_, err = builder.
		// observeDataTag
		NewFunctionBuilder().
		WithGoFunction(api.GoFunc(r.observeDataTag), []api.ValueType{i32}, []api.ValueType{}).
		Export(WasmFuncObserveDataTag).
		// write
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(r.write), []api.ValueType{i32, i32, i32}, []api.ValueType{i32}).
		Export(WasmFuncWrite).
		// context tag
		NewFunctionBuilder().
		WithGoFunction(api.GoFunc(r.contextTag), []api.ValueType{}, []api.ValueType{i32}).
		Export(WasmFuncContextTag).
		// context data
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(r.contextData), []api.ValueType{i32, i32}, []api.ValueType{i32}).
		Export(WasmFuncContextData).
		// context data size
		NewFunctionBuilder().
		WithGoFunction(api.GoFunc(r.contextDataSize), []api.ValueType{}, []api.ValueType{i32}).
		Export(WasmFuncContextDataSize).
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
	// context
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	default:
	}
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

func (r *wazeroRuntime) observeDataTag(ctx context.Context, stack []uint64) {
	tag := uint32(stack[0])
	r.observed = append(r.observed, tag)
}

func (r *wazeroRuntime) write(ctx context.Context, m api.Module, stack []uint64) {
	tag := uint32(stack[0])
	pointer := uint32(stack[1])
	length := uint32(stack[2])
	output, ok := m.Memory().Read(pointer, length)
	if !ok {
		log.Printf("Memory.Read(%d, %d) out of range\n", pointer, length)
		stack[0] = 1
		return
	}
	buf := make([]byte, length)
	copy(buf, output)

	if err := r.serverlessCtx.Write(tag, buf); err != nil {
		stack[0] = 2
		return
	}
	stack[0] = 0
}

func (r *wazeroRuntime) contextTag(ctx context.Context, stack []uint64) {
	stack[0] = uint64(r.serverlessCtx.Tag())
}

func (r *wazeroRuntime) contextData(ctx context.Context, m api.Module, stack []uint64) {
	pointer := uint32(stack[0])
	limit := uint32(stack[1])
	data := r.serverlessCtx.Data()
	dataLen := uint32(len(data))
	if dataLen > limit {
		stack[0] = uint64(dataLen)
		return
	} else if dataLen == 0 {
		stack[0] = 0
		return
	}
	if ok := m.Memory().Write(pointer, data); !ok {
		log.Printf("Memory.Write(%d, %d) out of range\n", pointer, dataLen)
		stack[0] = 0
		return
	}
	stack[0] = uint64(dataLen)
}

func (r *wazeroRuntime) contextDataSize(ctx context.Context, stack []uint64) {
	stack[0] = uint64(len(r.serverlessCtx.Data()))
}
