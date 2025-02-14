package provider

import (
	"context"
	"errors"
	"sync"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
)

// ErrNotExistsProvider is the error when the provider does not exist
var ErrNotExistsProvider = errors.New("llm provider does not exist")

// LLMProvider provides an interface to the llm providers
type LLMProvider interface {
	// Name returns the name of the llm provider
	Name() string
	// GetChatCompletions returns the chat completions.
	GetChatCompletions(context.Context, openai.ChatCompletionRequest, metadata.M) (openai.ChatCompletionResponse, error)
	// GetChatCompletionsStream returns the chat completions in stream.
	GetChatCompletionsStream(context.Context, openai.ChatCompletionRequest, metadata.M) (ResponseRecver, error)
}

// ResponseRecver receives stream response.
type ResponseRecver interface {
	// Recv is the receive function.
	Recv() (response openai.ChatCompletionStreamResponse, err error)
}

var (
	providers sync.Map
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

func getProvider(name string) LLMProvider {
	if provider, ok := providers.Load(name); ok {
		return provider.(LLMProvider)
	}
	return nil
}

// GetProvider returns the llm provider by name,
// if name is empty, it will return the first provider that has been registered
func GetProvider(name string) (LLMProvider, error) {
	if name == "" {
		var provider LLMProvider
		providers.Range(func(key, _ any) bool {
			name = key.(string)
			provider = getProvider(name)
			return false
		})
		if provider != nil {
			return provider, nil
		}
		return nil, ErrNotExistsProvider
	}
	provider := getProvider(name)
	if provider != nil {
		return provider, nil
	}
	return nil, ErrNotExistsProvider
}
