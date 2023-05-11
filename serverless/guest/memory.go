// guest wasm application programming interface for guest module
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
