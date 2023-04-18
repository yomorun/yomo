//go:build wasmtime

// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"fmt"
	"os"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/yomorun/yomo"
)

type wasmtimeRuntime struct {
	linker  *wasmtime.Linker
	store   *wasmtime.Store
	memory  *wasmtime.Memory
	init    *wasmtime.Func
	handler *wasmtime.Func

	observed  []yomo.Tag
	input     []byte
	outputTag yomo.Tag
	output    []byte
}

func newWasmtimeRuntime() (*wasmtimeRuntime, error) {
	engine := wasmtime.NewEngine()
	linker := wasmtime.NewLinker(engine)
	if err := linker.DefineWasi(); err != nil {
		return nil, fmt.Errorf("linker.DefineWasi: %v", err)
	}
	wasiConfig := wasmtime.NewWasiConfig()
	wasiConfig.InheritEnv()
	wasiConfig.InheritStdin()
	wasiConfig.InheritStdout()
	wasiConfig.InheritStderr()
	wasiConfig.PreopenDir(".", ".")
	store := wasmtime.NewStore(engine)
	store.SetWasi(wasiConfig)

	return &wasmtimeRuntime{
		linker: linker,
		store:  store,
	}, nil
}

// Init loads the wasm file, and initialize the runtime environment
func (r *wasmtimeRuntime) Init(wasmFile string) error {
	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		return fmt.Errorf("read wasm file %s: %v", wasmBytes, err)
	}

	module, err := wasmtime.NewModule(r.store.Engine, wasmBytes)
	if err != nil {
		return fmt.Errorf("wasmtime.NewModule: %v", err)
	}

	if err := r.linker.FuncWrap("env", WasmFuncObserveDataTag, r.observeDataTag); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncObserveDataTag, err)
	}

	if err := r.linker.FuncWrap("env", WasmFuncLoadInput, r.loadInput); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncLoadInput, err)
	}

	if err := r.linker.FuncWrap("env", WasmFuncDumpOutput, r.dumpOutput); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncDumpOutput, err)
	}

	instance, err := r.linker.Instantiate(r.store, module)
	if err != nil {
		return fmt.Errorf("wasmtime.NewInstance: %v", err)
	}
	r.memory = instance.GetExport(r.store, "memory").Memory()

	r.init = instance.GetFunc(r.store, WasmFuncInit)
	r.handler = instance.GetFunc(r.store, WasmFuncHandler)

	if _, err := r.init.Call(r.store); err != nil {
		return fmt.Errorf("init.Call %s: %v", WasmFuncInit, err)
	}

	return nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wasmtimeRuntime) GetObserveDataTags() []yomo.Tag {
	return r.observed
}

// RunHandler runs the wasm application (request -> response mode)
func (r *wasmtimeRuntime) RunHandler(data []byte) (yomo.Tag, []byte, error) {
	r.input = data
	// reset output
	r.outputTag = 0
	r.output = nil

	// run handler
	if _, err := r.handler.Call(r.store, len(data)); err != nil {
		return 0, nil, fmt.Errorf("handler.Call: %v", err)
	}

	return r.outputTag, r.output, nil
}

// Close releases all the resources related to the runtime
func (r *wasmtimeRuntime) Close() error {
	return nil
}

func (r *wasmtimeRuntime) observeDataTag(tag int32) {
	r.observed = append(r.observed, yomo.Tag(uint32(tag)))
}

func (r *wasmtimeRuntime) loadInput(pointer int32) {
	copy(r.memory.UnsafeData(r.store)[pointer:pointer+int32(len(r.input))], r.input)
}

func (r *wasmtimeRuntime) dumpOutput(tag int32, pointer int32, length int32) {
	r.outputTag = yomo.Tag(uint32(tag))
	r.output = make([]byte, length)
	copy(r.output, r.memory.UnsafeData(r.store)[pointer:pointer+length])
}
