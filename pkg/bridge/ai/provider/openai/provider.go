// Package openai is the OpenAI llm provider
package openai

import (
	"context"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"

	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// APIEndpoint is the endpoint for OpenAI
const APIEndpoint = "https://api.openai.com/v1/chat/completions"

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
var _ bridgeai.LLMProvider = &Provider{}

// NewProvider creates a new OpenAIProvider
func NewProvider(apiKey string, model string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}

	ylog.Debug("new openai provider", "api_endpoint", APIEndpoint, "api_key", apiKey, "model", model)
	return &Provider{
		APIKey: apiKey,
		Model:  model,
		client: openai.NewClient(apiKey),
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "openai"
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(req openai.ChatCompletionRequest, _ metadata.M) (openai.ChatCompletionResponse, error) {
	req.Model = p.Model

	return p.client.CreateChatCompletion(context.Background(), req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(req openai.ChatCompletionRequest, _ metadata.M) (*openai.ChatCompletionStream, error) {
	req.Model = p.Model

	return p.client.CreateChatCompletionStream(context.Background(), req)
}
