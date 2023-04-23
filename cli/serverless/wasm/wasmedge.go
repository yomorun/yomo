//go:build wasmedge

// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"fmt"
	"os"

	"github.com/second-state/WasmEdge-go/wasmedge"
)

type wasmEdgeRuntime struct {
	vm     *wasmedge.VM
	conf   *wasmedge.Configure
	module *wasmedge.Module

	observed  []uint32
	input     []byte
	outputTag uint32
	output    []byte
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

	observeDataTagFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{
			wasmedge.ValType_I32,
		},
		[]wasmedge.ValType{}), r.observeDataTag, nil, 0)
	r.module.AddFunction(WasmFuncObserveDataTag, observeDataTagFunc)

	loadInputFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{
			wasmedge.ValType_I32,
		},
		[]wasmedge.ValType{}), r.loadInput, nil, 0)
	r.module.AddFunction(WasmFuncLoadInput, loadInputFunc)

	dumpOutputFunc := wasmedge.NewFunction(wasmedge.NewFunctionType(
		[]wasmedge.ValType{
			wasmedge.ValType_I32,
			wasmedge.ValType_I32,
			wasmedge.ValType_I32,
		},
		[]wasmedge.ValType{}), r.dumpOutput, nil, 0)
	r.module.AddFunction(WasmFuncDumpOutput, dumpOutputFunc)

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

	_, err = r.vm.Execute(WasmFuncInit)
	if err != nil {
		return fmt.Errorf("vm.Execute %s: %v", WasmFuncInit, err)
	}

	return nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wasmEdgeRuntime) GetObserveDataTags() []uint32 {
	return r.observed
}

// RunHandler runs the wasm application (request -> response mode)
func (r *wasmEdgeRuntime) RunHandler(data []byte) (uint32, []byte, error) {
	r.input = data
	// reset output
	r.outputTag = 0
	r.output = nil

	// Run the handler function. Given the pointer to the input data.
	if _, err := r.vm.Execute(WasmFuncHandler, int32(len(data))); err != nil {
		return 0, nil, fmt.Errorf("vm.Execute %s: %v", WasmFuncHandler, err)
	}

	return r.outputTag, r.output, nil
}

// Close releases all the resources related to the runtime
func (r *wasmEdgeRuntime) Close() error {
	r.module.Release()
	r.vm.Release()
	r.conf.Release()
	return nil
}

func (r *wasmEdgeRuntime) observeDataTag(_ any, _ *wasmedge.CallingFrame, params []any) ([]any, wasmedge.Result) {
	tag := params[0].(int32)
	r.observed = append(r.observed, uint32(tag))
	return nil, wasmedge.Result_Success
}

func (r *wasmEdgeRuntime) loadInput(_ any, callframe *wasmedge.CallingFrame, params []any) ([]any, wasmedge.Result) {
	pointer := params[0].(int32)
	mem := callframe.GetMemoryByIndex(0)
	if err := mem.SetData(r.input, uint(pointer), uint(len(r.input))); err != nil {
		return nil, wasmedge.Result_Fail
	}
	return nil, wasmedge.Result_Success
}

func (r *wasmEdgeRuntime) dumpOutput(_ any, callframe *wasmedge.CallingFrame, params []any) ([]any, wasmedge.Result) {
	tag := params[0].(int32)
	pointer := params[1].(int32)
	length := params[2].(int32)
	r.outputTag = uint32(tag)
	mem := callframe.GetMemoryByIndex(0)
	output, err := mem.GetData(uint(pointer), uint(length))
	if err != nil {
		return nil, wasmedge.Result_Fail
	}
	r.output = output
	return nil, wasmedge.Result_Success
}
