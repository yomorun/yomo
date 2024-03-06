package register

import (
	"sync"
	"sync/atomic"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
)

var defaultRegister atomic.Value

func init()                  { SetRegister(&register{}) }
func SetRegister(r Register) { defaultRegister.Store(r) }

func ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error) {
	return defaultRegister.Load().(*register).ListToolCalls(md)
}

func RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error {
	return defaultRegister.Load().(*register).RegisterFunction(tag, functionDefinition, connID, md)
}

func UnregisterFunction(name string, connID uint64) {
	defaultRegister.Load().(*register).UnregisterFunction(name, connID)
}

type connectedFn struct {
	connID    uint64
	tag       uint32
	toolCalls ai.ToolCall
}

type Register interface {
	ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error)
	RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error
	UnregisterFunction(name string, connID uint64)
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

func (r *register) UnregisterFunction(name string, connID uint64) {
	r.underlying.Delete(connID)
}
