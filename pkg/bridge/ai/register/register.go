package register

import (
	"sync"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
)

var (
	// mu protects defaultRegister
	mu              sync.Mutex
	defaultRegister Register
)

func init() {
	SetRegister(&register{})
}
func SetRegister(r Register) {
	mu.Lock()
	defer mu.Unlock()
	defaultRegister = r
}

func ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error) {
	return defaultRegister.ListToolCalls(md)
}

func RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error {
	return defaultRegister.RegisterFunction(tag, functionDefinition, connID, md)
}

func UnregisterFunction(connID uint64, md metadata.M) {
	defaultRegister.UnregisterFunction(connID, md)
}

type connectedFn struct {
	connID    uint64
	tag       uint32
	toolCalls ai.ToolCall
}

type Register interface {
	ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error)
	RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error
	UnregisterFunction(connID uint64, md metadata.M)
}

type register struct {
	underlying sync.Map
}

func (r *register) ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error) {
	result := make(map[uint32]ai.ToolCall)

	r.underlying.Range(func(_, value any) bool {
		fn := value.(*connectedFn)
		result[fn.tag] = fn.toolCalls
		return true
	})

	return result, nil
}

func (r *register) RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error {
	r.underlying.Store(connID, &connectedFn{
		connID: connID,
		tag:    tag,
		toolCalls: ai.ToolCall{
			Type:     "function",
			Function: functionDefinition,
		},
	})

	return nil
}

func (r *register) UnregisterFunction(connID uint64, _ metadata.M) {
	r.underlying.Delete(connID)
}
