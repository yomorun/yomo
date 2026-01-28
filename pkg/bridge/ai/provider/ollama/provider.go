// Package ollama is used to provide the Ollama service for YoMo Bridge.
package ollama

import (
	"context"
	"os"

	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for Ollama
type Provider struct {
	// ollama OpenAI compatibility api endpoint
	APIEndpoint string
	// Model is the default model for Ollama
	Model  string
	client *openai.Client
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (openai.ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.Model
	}
	return p.client.CreateChatCompletion(ctx, req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (provider.ResponseRecver, error) {
	if req.Model == "" {
		req.Model = p.Model
	}
	return p.client.CreateChatCompletionStream(ctx, req)
}

// Name implements ai.LLMProvider.
func (p *Provider) Name() string {
	return "ollama"
}

// NewProvider creates a new OllamaProvider
func NewProvider(apiEndpoint string, model string) *Provider {
	if apiEndpoint == "" {
		v, ok := os.LookupEnv("OLLAMA_API_ENDPOINT")
		if ok {
			apiEndpoint = v
		} else {
			apiEndpoint = "http://localhost:11434/v1"
		}
	}
	if model == "" {
		v, ok := os.LookupEnv("OLLAMA_MODEL")
		if ok {
			model = v
		}
	}
	config := openai.DefaultConfig("ollama")
	config.BaseURL = apiEndpoint
	return &Provider{
		APIEndpoint: apiEndpoint,
		Model:       model,
		client:      openai.NewClientWithConfig(config),
	}
}
