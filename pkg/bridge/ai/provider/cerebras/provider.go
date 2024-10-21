// Package cerebras is the Cerebras llm provider
package cerebras

import (
	"context"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

const BaseURL = "https://api.cerebras.ai/v1"

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for Cerebras
type Provider struct {
	// APIKey is the API key for Cerberas
	APIKey string
	// Model is the model for Cerberas
	// eg. "llama3.1-8b", "llama-3.1-70b"
	Model  string
	client *openai.Client
}

// NewProvider creates a new cerebras ai provider
func NewProvider(apiKey string, model string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("CEREBRAS_API_KEY")
		if apiKey == "" {
			ylog.Error("CEREBRAS_API_KEY is empty, cerebras provider will not work properly")
		}
	}
	if model == "" {
		model = os.Getenv("CEREBRAS_MODEL")
		if model == "" {
			model = "llama3.1-8b"
		}
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
	return "cerebras"
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
	// The following fields are currently not supported and will result in a 400 error if they are supplied:
	// frequency_penalty
	// logit_bias
	// logprobs
	// presence_penalty
	// parallel_tool_calls
	// service_tier

	// it does not support streaming calls when tools are present

	return p.client.CreateChatCompletionStream(ctx, req)
}
