// Package register provides a register for registering and unregistering functions
package register

import (
	"fmt"
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

// NewDefault creates a new default register.
func NewDefault() Register {
	return &register{}
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

type register struct {
	underlying sync.Map
}

func (r *register) ListToolCalls(_ metadata.M) ([]openai.Tool, error) {
	result := []openai.Tool{}

	r.underlying.Range(func(_, value any) bool {
		tool := value.(openai.Tool)
		result = append(result, tool)
		return true
	})

	return result, nil
}

func (r *register) RegisterFunction(fd *ai.FunctionDefinition, connID uint64, md metadata.M) error {
	var err error
	r.underlying.Range(func(_, value any) bool {
		tool := value.(openai.Tool)
		if tool.Function.Name == fd.Name {
			err = fmt.Errorf("function %s already registered", fd.Name)
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	r.underlying.Store(connID, openai.Tool{
		Function: fd,
		Type:     openai.ToolTypeFunction,
	})

	return nil
}

func (r *register) UnregisterFunction(connID uint64, _ metadata.M) {
	r.underlying.Delete(connID)
}
