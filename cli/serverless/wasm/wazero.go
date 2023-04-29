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
	conf       wazero.ModuleConfig
	runConf    wazero.RuntimeConfig
	ctx        context.Context
	observed   []uint32
	guestBytes []byte
}

type wazeroInstance struct {
	ctx       context.Context
	input     []byte
	outputTag uint32
	output    []byte
	module    api.Module
	runtime   wazero.Runtime
}

func newWazeroRuntime() (*wazeroRuntime, error) {
	ctx := context.Background()
	cache := wazero.NewCompilationCache()
	runConfig := wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(true)
	config := newModuleConfig()
	r := newRuntime(ctx, runConfig)

	return &wazeroRuntime{
		Runtime: r,
		conf:    config,
		runConf: runConfig,
		ctx:     ctx,
	}, nil
}

func newModuleConfig() wazero.ModuleConfig {
	return wazero.NewModuleConfig().
		WithSysWalltime().
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)
}

func newRuntime(ctx context.Context, runConfig wazero.RuntimeConfig) wazero.Runtime {
	r := wazero.NewRuntimeWithConfig(ctx, runConfig)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	return r
}

// Init loads the wasm file, and initialize the runtime environment
func (r *wazeroRuntime) Init(wasmFile string) error {
	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		return fmt.Errorf("read wasm file %s: %v", wasmBytes, err)
	}
	r.guestBytes = wasmBytes
	// only used to compile host module
	i := &wazeroInstance{ctx: r.ctx}
	// host module
	builder := r.NewHostModuleBuilder("env")
	_, err = builder.
		// observeDataTag
		NewFunctionBuilder().
		WithFunc(r.observeDataTag).
		Export(WasmFuncObserveDataTag).
		// loadInput
		NewFunctionBuilder().
		WithFunc(i.loadInput).
		Export(WasmFuncLoadInput).
		// dumpOutput
		NewFunctionBuilder().
		WithFunc(i.dumpOutput).
		Export(WasmFuncDumpOutput).
		Instantiate(i.ctx)
	if err != nil {
		return fmt.Errorf("wazero.hostModule: %v", err)
	}
	// guest module
	guestModule, err := r.CompileModule(r.ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("wazero.compile: %v", err)
	}
	defer guestModule.Close(r.ctx)
	// guest
	module, err := r.InstantiateModule(i.ctx, guestModule, r.conf)
	if err != nil {
		return fmt.Errorf("wazero.guestModule: %v", err)
	}
	defer module.Close(i.ctx)
	// yomo init
	// WARN: this instance is only used to get observed tags,
	// running sfn handler must use runtime.instance
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

func (r *wazeroRuntime) Close() error {
	return r.Runtime.Close(r.ctx)
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wazeroRuntime) GetObserveDataTags() []uint32 {
	return r.observed
}

func (r *wazeroRuntime) observeDataTag(ctx context.Context, tag int32) {
	r.observed = append(r.observed, uint32(tag))
}

// ================================================================================

// Instance returns the wasm module instance
func (r *wazeroRuntime) Instance(ctx context.Context) (Instance, error) {
	// new runtime
	runtime := newRuntime(ctx, r.runConf)
	// instance
	i := &wazeroInstance{ctx: ctx}
	// host module
	builder := runtime.NewHostModuleBuilder("env")
	_, err := builder.
		// observeDataTag
		NewFunctionBuilder().
		WithFunc(r.observeDataTag).
		Export(WasmFuncObserveDataTag).
		// loadInput
		NewFunctionBuilder().
		WithFunc(i.loadInput).
		Export(WasmFuncLoadInput).
		// dumpOutput
		NewFunctionBuilder().
		WithFunc(i.dumpOutput).
		Export(WasmFuncDumpOutput).
		Instantiate(i.ctx)
	if err != nil {
		return nil, fmt.Errorf("instance.hostModule: %v", err)
	}
	// guest module
	guestModule, err := runtime.CompileModule(i.ctx, r.guestBytes)
	if err != nil {
		return nil, fmt.Errorf("instance.compile: %v", err)
	}
	module, err := runtime.InstantiateModule(i.ctx, guestModule, newModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("instance.guestMmodule: %v", err)
	}
	i.module = module
	i.runtime = runtime
	return i, nil
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
	i.module.Close(i.ctx)
	return i.runtime.Close(i.ctx)
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
