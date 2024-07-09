// Package openai is the OpenAI llm provider
package openai

import (
	"context"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// Provider is the provider for OpenAI
type Provider struct {
	// APIKey is the API key for OpenAI
	APIKey string
	// Model is the model for OpenAI
	// eg. "gpt-3.5-turbo-1106", "gpt-4-turbo-preview", "gpt-4-vision-preview", "gpt-4"
	Model  string
	client *openai.Client
}

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new OpenAIProvider
func NewProvider(apiKey string, model string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	c := openai.DefaultConfig(apiKey)
	if v, ok := os.LookupEnv("OPENAI_BASE_URL"); ok {
		c.BaseURL = v
	}

	return &Provider{
		APIKey: apiKey,
		Model:  model,
		client: openai.NewClientWithConfig(c),
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "openai"
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (openai.ChatCompletionResponse, error) {
	if p.Model != "" {
		req.Model = p.Model
	}

	return p.client.CreateChatCompletion(ctx, req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (provider.ResponseRecver, error) {
	if p.Model != "" {
		req.Model = p.Model
	}

	return p.client.CreateChatCompletionStream(ctx, req)
}
