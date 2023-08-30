//go:build wasmtime

// Package wasm provides WebAssembly serverless function runtimes.
package wasm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/bytecodealliance/wasmtime-go/v9"
	wasmhttp "github.com/yomorun/yomo/cli/serverless/wasm/http"

	"github.com/yomorun/yomo/serverless"
)

type wasmtimeRuntime struct {
	linker          *wasmtime.Linker
	store           *wasmtime.Store
	memory          *wasmtime.Memory
	init            *wasmtime.Func
	observeDataTags *wasmtime.Func
	handler         *wasmtime.Func

	observed      []uint32
	serverlessCtx serverless.Context
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
	// observeDataTag
	if err := r.linker.FuncWrap("env", WasmFuncObserveDataTag, r.observeDataTag); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncObserveDataTag, err)
	}
	// context tag
	if err := r.linker.FuncWrap("env", WasmFuncContextTag, r.contextTag); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncContextTag, err)
	}
	// context data
	if err := r.linker.FuncWrap("env", WasmFuncContextData, r.contextData); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncContextData, err)
	}
	// context data size
	if err := r.linker.FuncWrap("env", WasmFuncContextDataSize, r.contextDataSize); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncContextDataSize, err)
	}
	// write
	if err := r.linker.FuncWrap("env", WasmFuncWrite, r.write); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", WasmFuncWrite, err)
	}
	// http
	if err := r.linker.FuncWrap("env", wasmhttp.WasmFuncHTTPSend, r.httpSend); err != nil {
		return fmt.Errorf("linker.FuncWrap: %s %v", wasmhttp.WasmFuncHTTPSend, err)
	}
	// instantiate
	instance, err := r.linker.Instantiate(r.store, module)
	if err != nil {
		return fmt.Errorf("wasmtime.NewInstance: %v", err)
	}
	// memory
	r.memory = instance.GetExport(r.store, "memory").Memory()
	// _start
	startFunc := instance.GetExport(r.store, WasmFuncStart)
	if startFunc != nil {
		if _, err := startFunc.Func().Call(r.store); err != nil {
			return fmt.Errorf("start.Call %s: %v", WasmFuncInit, err)
		}
	}
	// yomo init and handler
	r.init = instance.GetFunc(r.store, WasmFuncInit)
	r.observeDataTags = instance.GetFunc(r.store, WasmFuncObserveDataTags)
	r.handler = instance.GetFunc(r.store, WasmFuncHandler)

	if r.observeDataTags == nil {
		return fmt.Errorf("%s function not found", WasmFuncObserveDataTags)
	}
	if _, err := r.observeDataTags.Call(r.store); err != nil {
		return fmt.Errorf("%s.Call: %v", WasmFuncObserveDataTags, err)
	}

	return nil
}

// GetObserveDataTags returns observed datatags of the wasm sfn
func (r *wasmtimeRuntime) GetObserveDataTags() []uint32 {
	return r.observed
}

// RunHandler runs the wasm application (request -> response mode)
func (r *wasmtimeRuntime) RunHandler(ctx serverless.Context) error {
	r.serverlessCtx = ctx
	// run handler
	if _, err := r.handler.Call(r.store); err != nil {
		return fmt.Errorf("handler.Call: %v", err)
	}
	return nil
}

// Close releases all the resources related to the runtime
func (r *wasmtimeRuntime) Close() error {
	return nil
}

// RunInitruns the init function of the wasm sfn
func (r *wasmtimeRuntime) RunInit() error {
	if r.init == nil {
		fmt.Println("init function not used")
		return nil
	}
	result, err := r.init.Call(r.store)
	if err != nil {
		return fmt.Errorf("init.Call: %v", err)
	}
	if result.(int32) != 0 {
		return errors.New("sfn initialization failed")
	}
	return nil
}

func (r *wasmtimeRuntime) observeDataTag(tag int32) {
	r.observed = append(r.observed, uint32(tag))
}

func (r *wasmtimeRuntime) contextTag() int32 {
	return int32(r.serverlessCtx.Tag())
}

func (r *wasmtimeRuntime) contextData(pointer int32, limit int32) (dataLen int32) {
	data := r.serverlessCtx.Data()
	dataLen = int32(len(data))
	if dataLen > limit {
		return
	} else if dataLen == 0 {
		return
	}
	copy(r.memory.UnsafeData(r.store)[pointer:pointer+int32(len(data))], data)
	return
}

func (r *wasmtimeRuntime) contextDataSize() int32 {
	return int32(len(r.serverlessCtx.Data()))
}

func (r *wasmtimeRuntime) write(tag int32, pointer int32, length int32) int32 {
	output := r.memory.UnsafeData(r.store)[pointer : pointer+length]
	if len(output) == 0 {
		return 0
	}
	buf := make([]byte, length)
	copy(buf, output)
	if err := r.serverlessCtx.Write(uint32(tag), buf); err != nil {
		return 2
	}
	return 0
}

// httpSend sends a HTTP request and returns the response
func (r *wasmtimeRuntime) httpSend(
	caller *wasmtime.Caller,
	reqPtr int32,
	reqSize int32,
	respPtr int32,
	respSize int32,
) int32 {
	if r.memory == nil {
		log.Printf("[HTTP] Send: memory is nil\n")
		return 1
	}
	// request
	reqBuf := r.memory.UnsafeData(r.store)[reqPtr : reqPtr+reqSize]
	respBuf, err := wasmhttp.Do(reqBuf)
	if err != nil {
		log.Printf("[HTTP] Send: %s\n", err)
		return 2
	}
	// write response
	allocFn := caller.GetExport("yomo_alloc")
	if allocFn == nil {
		log.Printf("[HTTP] Send: yomo_alloc not found\n")
		return 3
	}
	allocResult, err := allocFn.Func().Call(r.store, len(respBuf))
	if err != nil {
		log.Printf("[HTTP] Send: yomo_alloc error: %s\n", err)
		return 4
	}
	allocPtr32 := allocResult.(int32)
	allocPtr := int(allocPtr32)
	dataLen := len(respBuf)
	binary.LittleEndian.PutUint32(r.memory.UnsafeData(r.store)[respPtr:], uint32(allocPtr))
	binary.LittleEndian.PutUint32(r.memory.UnsafeData(r.store)[respSize:], uint32(len(respBuf)))
	copy(r.memory.UnsafeData(r.store)[allocPtr:allocPtr+dataLen], respBuf)
	return 0
}
