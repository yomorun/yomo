package ai

import (
	"sync"

	"github.com/yomorun/yomo/ai"
)

// LLMProvider provides an interface to the llm providers
type LLMProvider interface {
	// Name returns the name of the llm provider
	Name() string
	// GetOverview returns the overview of the AI functions, key is the tag, value is the function definition
	GetOverview() (*ai.OverviewResponse, error)
	// GetChatCompletions returns the chat completions
	GetChatCompletions(prompt string) (*ai.InvokeResponse, error)
	// RegisterFunction registers the llm function
	RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64) error
	// UnregisterFunction unregister the llm function
	UnregisterFunction(name string, connID uint64) error
	// ListToolCalls lists the llm tool calls
	ListToolCalls() (map[uint32]ai.ToolCall, error)
}

var (
	providers       sync.Map
	defaultProvider LLMProvider
	mu              sync.Mutex
)

// RegisterProvider registers the llm provider
func RegisterProvider(provider LLMProvider) {
	if provider != nil {
		providers.Store(provider.Name(), provider)
	}
}

// ListProviders returns the list of llm providers
func ListProviders() []string {
	var names []string
	providers.Range(func(key, _ any) bool {
		names = append(names, key.(string))
		return true
	})
	return names
}

// SetDefaultProvider sets the default llm provider
func SetDefaultProvider(name string) {
	provider := GetProvider(name)
	if provider != nil {
		setDefaultProvider(provider)
	}
}

func setDefaultProvider(provider LLMProvider) {
	mu.Lock()
	defer mu.Unlock()
	defaultProvider = provider
}

// GetProvider returns the llm provider by name
func GetProvider(name string) LLMProvider {
	if provider, ok := providers.Load(name); ok {
		return provider.(LLMProvider)
	}
	return nil
}

// GetProviderAndSetDefault returns the llm provider by name and set it as the default provider
func GetProviderAndSetDefault(name string) (LLMProvider, error) {
	provider := GetProvider(name)
	if provider != nil {
		setDefaultProvider(provider)
		return provider, nil
	}
	return nil, ErrNotExistsProvider
}

// GetDefaultProvider returns the default llm provider
func GetDefaultProvider() (LLMProvider, error) {
	mu.Lock()
	defer mu.Unlock()
	if defaultProvider != nil {
		return defaultProvider, nil
	}
	names := ListProviders()
	if len(names) > 0 {
		p := GetProvider(names[0])
		if p != nil {
			return p, nil
		}
	}
	return nil, ErrNotExistsProvider
}
