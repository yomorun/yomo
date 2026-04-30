// Package mistral is the Mistral llm provider
package mistral

import (
	"context"

	"github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// based on https://docs.mistral.ai/api/
const BaseURL = "https://api.mistral.ai/v1"

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for mistral
type Provider struct {
	// APIKey is the API key for mistral
	APIKey string
	// Model is the model for mistral
	// eg. "mistral-tiny", "mistral-small", "mistral-medium"
	Model  string
	client *openai.Client
}

// NewProvider creates a new mistral ai provider
func NewProvider(apiKey string, model string) *Provider {
	if model == "" {
		model = "mistral-tiny"
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
	return "mistral"
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
