//go:build !wasmedge

// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import "errors"

func newWasmEdgeRuntime() (Runtime, error) {
	return nil, errors.New("this cli version doesn't support WasmEdge, please rebuild cli: TAGS=wasmedge make build")
}
