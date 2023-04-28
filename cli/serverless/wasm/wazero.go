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
)

type wazeroRuntime struct {
	wazero.Runtime
	conf        wazero.ModuleConfig
	ctx         context.Context
	cache       wazero.CompilationCache
	hostModule  wazero.CompiledModule
	guestModule wazero.CompiledModule
}

type wazeroInstance struct {
	ctx       context.Context
	observed  []uint32
	input     []byte
	outputTag uint32
	output    []byte
	module    api.Module
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
	i := &wazeroInstance{ctx: r.ctx}
	// i := r.instance
	// r.instance = i
	// host module
	builder := r.NewHostModuleBuilder("env")
	hostModule, err := builder.NewFunctionBuilder().
		// observeDataTag
		WithFunc(i.observeDataTag).
		Export(WasmFuncObserveDataTag).
		// loadInput
		NewFunctionBuilder().
		WithFunc(i.loadInput).
		Export(WasmFuncLoadInput).
		// dumpOutput
		NewFunctionBuilder().
		WithFunc(i.dumpOutput).
		Export(WasmFuncDumpOutput).
		// Instantiate(i.ctx)
		Compile(r.ctx)
	r.hostModule = hostModule
	// guest module
	// module, err := r.InstantiateWithConfig(r.ctx, wasmBytes, r.conf)
	guestModule, err := r.CompileModule(r.ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("wazero.Module: %v", err)
	}
	r.guestModule = guestModule

	// r.module = module

	// init := module.ExportedFunction(WasmFuncInit)

	// if _, err := init.Call(r.ctx); err != nil {
	// 	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
	// 		return fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
	// 	} else if !ok {
	// 		return fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
	// 	}
	// }

	return nil
}

func (r *wazeroRuntime) Close() error {
	r.hostModule.Close(r.ctx)
	r.guestModule.Close(r.ctx)
	return r.Runtime.Close(r.ctx)
}

// ================================================================================

// Instance returns the wasm module instance
func (r *wazeroRuntime) Instance() (Instance, error) {
	// instance
	i := &wazeroInstance{ctx: r.ctx}
	// i := r.instance
	// host module
	// builder := r.NewHostModuleBuilder("env")
	// _, err := builder.NewFunctionBuilder().
	// 	// observeDataTag
	// 	WithFunc(i.observeDataTag).
	// 	Export(WasmFuncObserveDataTag).
	// 	// loadInput
	// 	NewFunctionBuilder().
	// 	WithFunc(i.loadInput).
	// 	Export(WasmFuncLoadInput).
	// 	// dumpOutput
	// 	NewFunctionBuilder().
	// 	WithFunc(i.dumpOutput).
	// 	Export(WasmFuncDumpOutput).
	// 	Instantiate(i.ctx)
	// 	// Compile(r.ctx)
	// if err != nil {
	// 	return nil, fmt.Errorf("wazero.HostFunc: %v", err)
	// }
	_, err := r.InstantiateModule(i.ctx, r.hostModule, wazero.NewModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("wazero.hostModule: %v", err)
	}
	// guest module
	module, err := r.InstantiateModule(i.ctx, r.guestModule, r.conf)
	if err != nil {
		return nil, fmt.Errorf("wazero.guestModule: %v", err)
	}
	// yomo init
	init := module.ExportedFunction(WasmFuncInit)
	if _, err := init.Call(i.ctx); err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			return nil, fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
		} else if !ok {
			return nil, fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
		}
	}
	i.module = module
	return i, nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (i *wazeroInstance) GetObserveDataTags() []uint32 {
	return i.observed
}

// RunHandler runs the wasm application (request -> response mode)
func (i *wazeroInstance) RunHandler(data []byte) (uint32, []byte, error) {
	i.input = data
	// reset output
	i.outputTag = 0
	i.output = nil

	// run handler
	handler := i.module.ExportedFunction(WasmFuncHandler)
	if _, err := handler.Call(i.ctx, uint64(len(data))); err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			return 0, nil, fmt.Errorf("handler.Call: %v", err)
		} else if !ok {
			return 0, nil, fmt.Errorf("handler.Call: %v", err)
		}
	}

	return i.outputTag, i.output, nil
}

// Close releases all the resources related to the runtime
func (i *wazeroInstance) Close() error {
	return i.module.Close(i.ctx)
}

func (i *wazeroInstance) observeDataTag(ctx context.Context, tag int32) {
	i.observed = append(i.observed, uint32(tag))
}

func (i *wazeroInstance) loadInput(ctx context.Context, m api.Module, pointer int32) {
	memSize := m.Memory().Size()
	dataSize := uint32(int(pointer) + len(i.input))
	// log.Printf("loadInput: memSize=%d, dataSize=%d\n", memSize, dataSize)
	if dataSize > memSize {
		log.Printf("data size too big: %d, grow +%d\n", dataSize, dataSize-memSize)
		if _, ok := m.Memory().Grow(uint32(dataSize - memSize)); !ok {
			log.Panicf("Memory.Grow(%d) failed", dataSize-memSize)
		}
	}
	if !m.Memory().Write(uint32(pointer), i.input) {
		log.Panicf("Memory.Write(%d, %d) out of range of memory size %d",
			pointer, len(i.input), m.Memory().Size())
	}
}

func (i *wazeroInstance) dumpOutput(ctx context.Context, m api.Module, tag int32, pointer int32, length int32) {
	i.outputTag = uint32(tag)
	i.output = make([]byte, length)
	buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	}
	copy(i.output, buf)
}
