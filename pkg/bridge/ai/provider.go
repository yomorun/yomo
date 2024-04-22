package ai

import (
	"sync"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
)

// LLMProvider provides an interface to the llm providers
type LLMProvider interface {
	// Name returns the name of the llm provider
	Name() string
	// GetChatCompletions returns the chat completions.
	GetChatCompletions(openai.ChatCompletionRequest, metadata.M) (openai.ChatCompletionResponse, error)
	// GetChatCompletionsStream returns the chat completions in stream.
	GetChatCompletionsStream(openai.ChatCompletionRequest, metadata.M) (*openai.ChatCompletionStream, error)
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
