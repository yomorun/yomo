// Package groq is the Groq llm provider
package groq

import (
	"context"

	"github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// based on https://console.groq.com/docs/openai
const BaseURL = "https://api.groq.com/openai/v1"

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for groq
type Provider struct {
	// APIKey is the API key for groq
	APIKey string
	// Model is the model for groq
	// eg. "llama3-8b-8192", "llama3-70b-8192", "mixtral-8x7b-32768"
	Model  string
	client *openai.Client
}

// NewProvider creates a new groq ai provider
func NewProvider(apiKey string, model string) *Provider {
	if model == "" {
		model = "llama3-8b-8192"
	}
	c := openai.DefaultConfig(apiKey)
	c.BaseURL = BaseURL

	return &Provider{
		APIKey: apiKey,
		Model:  model,
		client: openai.NewClientWithConfig(c),
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "groq"
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
