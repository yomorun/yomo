package wazero

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// allocateBuffer allocates memory and writes the data to the memory
func allocateBuffer(
	ctx context.Context,
	m api.Module,
	bufPtr uint32,
	bufSize uint32,
	buf []byte,
) error {
	bufLen := len(buf)
	memResults, err := m.ExportedFunction("yomo_alloc").Call(ctx, uint64(bufLen))
	if err != nil {
		return err
	}
	allocPtr := uint32(memResults[0])
	if !m.Memory().WriteUint32Le(bufPtr, allocPtr) {
		return fmt.Errorf("memory write(%d) with %d out of range", bufPtr, allocPtr)
	}
	if !m.Memory().WriteUint32Le(bufSize, uint32(bufLen)) {
		return fmt.Errorf("memory write(%d) with %d out of range", bufSize, bufLen)
	}
	if !m.Memory().Write(allocPtr, buf) {
		return fmt.Errorf("memory write(%d, %d) out of range", allocPtr, bufLen)
	}
	return nil
}

func readBuffer(ctx context.Context, m api.Module, bufPtr uint32, bufSize uint32) ([]byte, error) {
	buf, ok := m.Memory().Read(bufPtr, bufSize)
	if !ok {
		return nil, fmt.Errorf("memory read(%d, %d) out of range", bufPtr, bufSize)
	}
	result := make([]byte, bufSize)
	copy(result, buf)
	return result, nil
}
