// Package vllm is the vllm llm provider
package vllm

import (
	"context"

	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for vllm
type Provider struct {
	// vllm OpenAI compatibility api endpoint
	APIEndpoint string
	// APIKey is the API key for vllm
	APIKey string
	// Model is the model for vllm
	// eg. "meta-llama/Llama-3.2-7B-Instruct"
	Model  string
	client *openai.Client
}

// NewProvider creates a new vllm ai provider
func NewProvider(apiEndpoint string, apiKey string, model string) *Provider {
	if apiEndpoint == "" {
		apiEndpoint = "http://127.0.0.1:8000"
	}
	// vllm api endpoint is different from the default openai api endpoint, so we need to append "/v1" to the endpoint
	apiEndpoint = apiEndpoint + "/v1"
	if model == "" {
		model = "meta-llama/Llama-3.2-7B-Instruct"
	}

	c := openai.DefaultConfig(apiKey)
	c.BaseURL = apiEndpoint

	return &Provider{
		APIEndpoint: apiEndpoint,
		APIKey:      apiKey,
		Model:       model,
		client:      openai.NewClientWithConfig(c),
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "vllm"
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
