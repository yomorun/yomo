// Package guest is the wasm application programming interface for guest module
package guest

import (
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

// bufferPtrSize returns the memory position and size of the buffer
func bufferToPtrSize(buff []byte) (uintptr, uint32) {
	ptr := &buff[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return unsafePtr, uint32(len(buff))
}

// readBufferFromMemory returns a buffer
func readBufferFromMemory(bufferPosition *uint32, length uint32) []byte {
	buf := make([]byte, length)
	ptr := uintptr(unsafe.Pointer(bufferPosition))
	for i := 0; i < int(length); i++ {
		s := *(*int32)(unsafe.Pointer(ptr + uintptr(i)))
		buf[i] = byte(s)
	}
	return buf
}

//export yomo_alloc
func alloc(size uint32) uintptr {
	buf := make([]byte, size)
	ptr := &buf[0]
	return uintptr(unsafe.Pointer(ptr))
}
