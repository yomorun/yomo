// Package cfopenai is used to provide the Azure OpenAI service
package cfopenai

import (
	"context"
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
)

// Provider is the provider for Cloudflare OpenAI Gateway
type Provider struct {
	// CfEndpoint is the your cloudflare endpoint
	CfEndpoint string
	// APIKey is the API key for OpenAI
	APIKey string
	// Model is the model for OpenAI
	Model  string
	client oai.OpenAIRequester
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
	// if cfEndpoint == "" {
	// 	ylog.Error("cfEndpoint is required")
	// }
	ylog.Debug("new cloudflare openai provider", "api_key", apiKey, "model", model, "cloudflare_endpoint", cfEndpoint)
	return &Provider{
		CfEndpoint: cfEndpoint,
		APIKey:     apiKey,
		Model:      model,
		client:     &oai.OpenAIClient{},
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "cloudflare_openai"
}

// GetChatCompletions get chat completions for ai service
func (p *Provider) GetChatCompletions(req *ai.ChatCompletionRequest) (*ai.ChatCompletionResponse, error) {
	req.Model = p.Model

	url := fmt.Sprintf("%s/openai/chat/completions", p.CfEndpoint)

	res, err := p.client.ChatCompletions(
		context.Background(),
		url,
		"Authorization",
		fmt.Sprintf("Bearer %s", p.APIKey),
		req,
	)

	return res, err
}
