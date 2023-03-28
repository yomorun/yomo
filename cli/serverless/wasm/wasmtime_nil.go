//go:build !wasmtime

// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import "errors"

func newWasmtimeRuntime() (Runtime, error) {
	return nil, errors.New("this cli version doesn't support Wasmtime, please rebuild cli: TAGS=wastime make build")
}
