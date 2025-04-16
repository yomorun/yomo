package ai

import (
	"sync"

	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
)

var (
	mu              sync.Mutex
	defaultRegister Register
)

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
func ListToolCalls(md metadata.M) ([]openai.Tool, error) {
	return defaultRegister.ListToolCalls(md)
}

// RegisterFunction registers a function calling function
func RegisterFunction(functionDefinition *openai.FunctionDefinition, connID uint64, md metadata.M) error {
	return defaultRegister.RegisterFunction(functionDefinition, connID, md)
}

// UnregisterFunction unregisters a function calling function
func UnregisterFunction(connID uint64, md metadata.M) {
	defaultRegister.UnregisterFunction(connID, md)
}

// Register provides an stateful register for registering and unregistering functions
type Register interface {
	// ListToolCalls returns the list of tool calls
	ListToolCalls(md metadata.M) ([]openai.Tool, error)
	// RegisterFunction registers a function calling function
	RegisterFunction(fd *openai.FunctionDefinition, connID uint64, md metadata.M) error
	// UnregisterFunction unregisters a function calling function
	UnregisterFunction(connID uint64, md metadata.M)
}
