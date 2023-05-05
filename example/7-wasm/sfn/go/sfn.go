package main

import (
	"fmt"
	"strings"
	"unsafe"
)

var (
	// ReadBuf is a buffer used to read data from the host.
	ReadBuf = make([]byte, ReadBufSize)
	// ReadBufPtr is a pointer to ReadBuf.
	ReadBufPtr = uintptr(unsafe.Pointer(&ReadBuf[0]))
	// ReadBufSize is the size of ReadBuf
	ReadBufSize = uint32(2048)
)

// GetBytes returns a byte slice of the given size
func GetBytes(fn func(ptr uintptr, size uint32) (len uint32)) (result []byte) {
	size := fn(ReadBufPtr, ReadBufSize)
	if size == 0 {
		return
	}
	if size > 0 && size <= ReadBufSize {
		// copy to avoid passing a mutable buffer
		result = make([]byte, size)
		copy(result, ReadBuf)
		return
	}
	// Otherwise, allocate a new buffer
	buf := make([]byte, size)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	_ = fn(ptr, size)
	return buf
}

func ContextData(ptr uintptr, size uint32) uint32 {
	return contextData(ptr, size)
}

func main() {}

//export yomo_observe_datatag
func yomoObserveDataTag(tag uint32)

//export yomo_write
func yomoWrite(tag uint32, pointer *byte, length int) uint32

//export yomo_context_tag
func yomoContextTag() uint32

//export yomo_context_data
func contextData(ptr uintptr, size uint32) uint32

//export yomo_init
func yomoInit() {
	yomoObserveDataTag(0x33)
}

//export yomo_handler
func yomoHandler() {
	// load input data
	tag := yomoContextTag()
	input := GetBytes(ContextData)
	fmt.Printf("wasm go sfn received %d bytes with tag[%#x]\n", len(input), tag)

	// process app data
	output := strings.ToUpper(string(input))

	// dump output data
	yomoWrite(0x34, &[]byte(output)[0], len(output))
}
