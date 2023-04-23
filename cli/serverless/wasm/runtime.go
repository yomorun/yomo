// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"fmt"
)

// Define wasm import/export function names
const (
	WasmFuncInit           = "yomo_init"
	WasmFuncObserveDataTag = "yomo_observe_datatag"
	WasmFuncLoadInput      = "yomo_load_input"
	WasmFuncDumpOutput     = "yomo_dump_output"
	WasmFuncHandler        = "yomo_handler"
)

// Runtime is the abstract interface for wasm runtime
type Runtime interface {
	// Init loads the wasm file, and initialize the runtime environment
	Init(wasmFile string) error

	// GetObserveDataTags returns observed datatags of the wasm sfn
	GetObserveDataTags() []uint32

	// RunHandler runs the wasm application (request -> response mode)
	RunHandler(data []byte) (uint32, []byte, error)

	// Close releases all the resources related to the runtime
	Close() error
}

// NewRuntime returns a specific wasm runtime instance according to the type parameter
func NewRuntime(runtimeType string) (Runtime, error) {
	switch runtimeType {
	case "", "wazero":
		return newWazeroRuntime()
	case "wasmtime":
		return newWasmtimeRuntime()
	case "wasmedge":
		return newWasmEdgeRuntime()
	default:
		return nil, fmt.Errorf("invalid runtime type: %s, wasmtime and wasmedge are supported in current version", runtimeType)
	}
}
