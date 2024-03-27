// Package register provides a register for registering and unregistering functions
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

// SetRegister sets the default register
func SetRegister(r Register) {
	mu.Lock()
	defer mu.Unlock()
	defaultRegister = r
}

// ListToolCalls returns the list of tool calls
func ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error) {
	return defaultRegister.ListToolCalls(md)
}

// RegisterFunction registers a function calling function
func RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error {
	return defaultRegister.RegisterFunction(tag, functionDefinition, connID, md)
}

// UnregisterFunction unregisters a function calling function
func UnregisterFunction(connID uint64, md metadata.M) {
	defaultRegister.UnregisterFunction(connID, md)
}

// SfnFactor returns the sfn factor
func SfnFactor(tag uint32) int {
	return defaultRegister.SfnFactor(tag)
}

type connectedFn struct {
	connID    uint64
	tag       uint32
	toolCalls ai.ToolCall
}

// Register provides an stateful register for registering and unregistering functions
type Register interface {
	// ListToolCalls returns the list of tool calls
	ListToolCalls(md metadata.M) (map[uint32]ai.ToolCall, error)
	// RegisterFunction registers a function calling function
	RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error
	// UnregisterFunction unregisters a function calling function
	UnregisterFunction(connID uint64, md metadata.M)
	// SfnFactor returns the sfn factor
	SfnFactor(tag uint32) int
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

// SfnFactor returns the sfn factor
func (r *register) SfnFactor(tag uint32) int {
	factor := 0
	r.underlying.Range(func(key, value any) bool {
		fn := value.(*connectedFn)
		if fn.tag == tag {
			factor++
		}
		return true
	})
	return factor
}
