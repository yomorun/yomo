// Package githubmodels is the Github Models llm provider, see https://github.com/marketplace/models
package githubmodels

import (
	"context"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	"github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// Provider is the provider for Github Models
type Provider struct {
	// APIKey is the API key for Github Models
	APIKey string
	// Model is the model for Github Models, see https://github.com/marketplace/models
	// e.g. "Meta-Llama-3.1-405B-Instruct", "Mistral-large-2407", "gpt-4o"
	Model  string
	client *openai.Client
}

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new OpenAIProvider
func NewProvider(apiKey string, model string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("GITHUB_TOKEN")
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = "https://models.inference.ai.azure.com"

	return &Provider{
		APIKey: apiKey,
		Model:  model,
		client: openai.NewClientWithConfig(config),
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "githubmodels"
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
