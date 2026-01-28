// Package xai is the x.ai provider
package xai

import (
	"context"

	_ "github.com/joho/godotenv/autoload"
	"github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

const BaseURL = "https://api.x.ai/v1"
const DefaultModel = "grok-beta"

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for x.ai
type Provider struct {
	// APIKey is the API key for x.ai
	APIKey string
	// Model is the model for x.ai
	// eg. "grok-beta"
	Model  string
	client *openai.Client
}

// NewProvider creates a new x.ai ai provider
func NewProvider(apiKey string, model string) *Provider {
	if model == "" {
		model = DefaultModel
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
	return "xai"
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
	// it does not support streaming calls when tools are present
	return p.client.CreateChatCompletionStream(ctx, req)
}
