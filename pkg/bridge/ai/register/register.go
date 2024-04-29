// Package register provides a register for registering and unregistering functions
package register

import (
	"sync"

	"github.com/sashabaranov/go-openai"
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

// GetRegister gets the default register
func GetRegister() Register {
	mu.Lock()
	defer mu.Unlock()
	return defaultRegister
}

// ListToolCalls returns the list of tool calls
func ListToolCalls(md metadata.M) (map[uint32]openai.Tool, error) {
	return defaultRegister.ListToolCalls(md)
}

// RegisterFunction registers a function calling function
func RegisterFunction(tag uint32, functionDefinition *openai.FunctionDefinition, connID uint64, md metadata.M) error {
	return defaultRegister.RegisterFunction(tag, functionDefinition, connID, md)
}

// UnregisterFunction unregisters a function calling function
func UnregisterFunction(connID uint64, md metadata.M) {
	defaultRegister.UnregisterFunction(connID, md)
}

// SfnFactor returns the sfn factor
func SfnFactor(tag uint32, md metadata.M) int {
	return defaultRegister.SfnFactor(tag, md)
}

type connectedFn struct {
	connID uint64
	tag    uint32
	tools  openai.Tool
}

// Register provides an stateful register for registering and unregistering functions
type Register interface {
	// ListToolCalls returns the list of tool calls
	ListToolCalls(md metadata.M) (map[uint32]openai.Tool, error)
	// RegisterFunction registers a function calling function
	RegisterFunction(tag uint32, functionDefinition *openai.FunctionDefinition, connID uint64, md metadata.M) error
	// UnregisterFunction unregisters a function calling function
	UnregisterFunction(connID uint64, md metadata.M)
	// SfnFactor returns the sfn factor
	SfnFactor(tag uint32, md metadata.M) int
}

type register struct {
	underlying sync.Map
}

func (r *register) ListToolCalls(md metadata.M) (map[uint32]openai.Tool, error) {
	result := make(map[uint32]openai.Tool)

	r.underlying.Range(func(_, value any) bool {
		fn := value.(*connectedFn)
		result[fn.tag] = fn.tools
		return true
	})

	return result, nil
}

func (r *register) RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64, md metadata.M) error {
	r.underlying.Store(connID, &connectedFn{
		connID: connID,
		tag:    tag,
		tools: openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: functionDefinition,
		},
	})

	return nil
}

func (r *register) UnregisterFunction(connID uint64, _ metadata.M) {
	r.underlying.Delete(connID)
}

// SfnFactor returns the sfn factor
func (r *register) SfnFactor(tag uint32, md metadata.M) int {
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
