// Package cfopenai is used to provide the Azure OpenAI service
package cfopenai

import (
	"context"
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	openai "github.com/sashabaranov/go-openai"

	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// Provider is the provider for Cloudflare OpenAI Gateway
type Provider struct {
	// CfEndpoint is the your cloudflare endpoint
	CfEndpoint string
	// APIKey is the API key for OpenAI
	APIKey string
	// Model is the model for OpenAI
	Model  string
	client *openai.Client
}

// check if implements ai.Provider
var _ bridgeai.LLMProvider = &Provider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(cfEndpoint, apiKey, model string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}

	client := openai.NewClientWithConfig(newConfig(apiKey, cfEndpoint))

	ylog.Debug("new cloudflare openai provider", "api_key", apiKey, "model", model, "cloudflare_endpoint", cfEndpoint)
	return &Provider{
		CfEndpoint: cfEndpoint,
		APIKey:     apiKey,
		Model:      model,
		client:     client,
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "cloudflare_openai"
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (openai.ChatCompletionResponse, error) {
	req.Model = p.Model

	return p.client.CreateChatCompletion(ctx, req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (*openai.ChatCompletionStream, error) {
	req.Model = p.Model

	return p.client.CreateChatCompletionStream(ctx, req)
}

func newConfig(apiKey, cfEndpoint string) openai.ClientConfig {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = fmt.Sprintf("%s/openai", cfEndpoint)

	return config
}
