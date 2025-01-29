// Package cerebras is the Cerebras llm provider
package cerebras

import (
	"context"

	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// based on https://api-docs.deepseek.com/
const BaseURL = "https://api.deepseek.com/v1"

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for Cerebras
type Provider struct {
	// APIKey is the API key for Cerebras
	APIKey string
	// Model is the model for Cerebras
	// eg. "deepseek-chat", "deepseek-reasoner"
	Model  string
	client *openai.Client
}

// NewProvider creates a new cerebras ai provider
func NewProvider(apiKey string, model string) *Provider {
	if model == "" {
		model = "deepseek-chat"
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
	return "deepseek"
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
