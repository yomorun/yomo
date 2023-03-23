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
	"github.com/yomorun/yomo/core/frame"
)

type wazeroRuntime struct {
	wazero.Runtime
	conf   wazero.ModuleConfig
	ctx    context.Context
	module api.Module

	observed  []frame.Tag
	input     []byte
	outputTag frame.Tag
	output    []byte
}

func newWazeroRuntime() (*wazeroRuntime, error) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	// Instantiate WASI, which implements host functions needed for TinyGo to implement `panic`.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().
		// WithStartFunctions().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)
	return &wazeroRuntime{
		Runtime: r,
		conf:    config,
		ctx:     ctx,
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
		return fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
	}

	return nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wazeroRuntime) GetObserveDataTags() []frame.Tag {
	return r.observed
}

// RunHandler runs the wasm application (request -> response mode)
func (r *wazeroRuntime) RunHandler(data []byte) (frame.Tag, []byte, error) {
	r.input = data
	// reset output
	r.outputTag = 0
	r.output = nil

	// run handler
	handler := r.module.ExportedFunction(WasmFuncHandler)
	if _, err := handler.Call(r.ctx, uint64(len(data))); err != nil {
		return 0, nil, fmt.Errorf("handler.Call: %v", err)
	}

	return r.outputTag, r.output, nil
}

// Close releases all the resources related to the runtime
func (r *wazeroRuntime) Close() error {
	return r.Runtime.Close(r.ctx)
}

func (r *wazeroRuntime) observeDataTag(ctx context.Context, tag int32) {
	r.observed = append(r.observed, frame.Tag(uint32(tag)))
}

func (r *wazeroRuntime) loadInput(ctx context.Context, m api.Module, pointer int32) {
	if !m.Memory().Write(uint32(pointer), r.input) {
		log.Panicf("Memory.Write(%d, %d) out of range of memory size %d",
			pointer, len(r.input), m.Memory().Size())
	}
}

func (r *wazeroRuntime) dumpOutput(ctx context.Context, m api.Module, tag int32, pointer int32, length int32) {
	r.outputTag = frame.Tag(uint32(tag))
	r.output = make([]byte, length)
	buf, ok := m.Memory().Read(uint32(pointer), uint32(length))
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", pointer, length)
	}
	copy(r.output, buf)
}
