package register

import (
	"fmt"
	"sync"

	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
)

type mcpToolStore interface {
	AddMCPTool(connID uint64, fd *ai.FunctionDefinition) error
	RemoveMCPTool(connID uint64) error
}

// NewDefault creates a new default register.
func NewDefault(mcpToolStore mcpToolStore) ai.Register {
	return &register{
		underlying:   sync.Map{},
		mcpToolStore: mcpToolStore,
	}
}

type register struct {
	underlying   sync.Map
	mcpToolStore mcpToolStore
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
			err = fmt.Errorf("function `%s` already registered", fd.Name)
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	// ai function
	r.underlying.Store(connID, openai.Tool{
		Function: fd,
		Type:     openai.ToolTypeFunction,
	})
	if r.mcpToolStore == nil {
		return nil
	}
	// mcp tool
	err = r.mcpToolStore.AddMCPTool(connID, fd)
	if err != nil {
		return err
	}

	return nil
}

func (r *register) UnregisterFunction(connID uint64, _ metadata.M) {
	// ai function
	r.underlying.Delete(connID)

	// mcp tool
	if r.mcpToolStore == nil {
		return
	}
	r.mcpToolStore.RemoveMCPTool(connID)
}
