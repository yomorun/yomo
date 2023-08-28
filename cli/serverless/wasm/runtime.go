// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"fmt"

	"github.com/yomorun/yomo/serverless"
)

// Define wasm import/export function names
const (
	WasmFuncStart = "_start"
	WasmFuncInit  = "yomo_init"
	// WasmFuncObserveDataTags guest module should implement this function
	WasmFuncObserveDataTags = "yomo_observe_datatags"
	// WasmFuncObserveDataTag host module should implement this function
	WasmFuncObserveDataTag  = "yomo_observe_datatag"
	WasmFuncHandler         = "yomo_handler"
	WasmFuncWrite           = "yomo_write"
	WasmFuncContextTag      = "yomo_context_tag"
	WasmFuncContextData     = "yomo_context_data"
	WasmFuncContextDataSize = "yomo_context_data_size"
)

// Runtime is the abstract interface for wasm runtime
type Runtime interface {
	// Init loads the wasm file, and initialize the runtime environment
	Init(wasmFile string) error

	// GetObserveDataTags returns observed datatags of the wasm sfn
	GetObserveDataTags() []uint32

	// RunInit runs the init function of the wasm sfn
	RunInit() error

	// RunHandler runs the wasm application (request -> response mode)
	RunHandler(ctx serverless.Context) error

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
