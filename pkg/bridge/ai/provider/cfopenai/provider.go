// Package cfopenai is used to provide the Azure OpenAI service
package cfopenai

import (
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/openai"
)

// CloudflareOpenaiProvider is the provider for Cloudflare OpenAI Gateway
type CloudflareOpenAIProvider struct {
	// CfEndpoint is the your cloudflare endpoint
	CfEndpoint string
	// APIKey is the API key for OpenAI
	APIKey string
	// Model is the model for OpenAI
	Model string
}

// check if implements ai.Provider
var _ bridgeai.LLMProvider = &CloudflareOpenAIProvider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(cfEndpoint, apiKey, model string) *CloudflareOpenAIProvider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	if cfEndpoint == "" {
		ylog.Error("cfEndpoint is required")
		os.Exit(-1)
	}
	ylog.Debug("new cloudflare openai provider", "api_key", apiKey, "model", model, "cloudflare_endpoint", cfEndpoint)
	return &CloudflareOpenAIProvider{
		CfEndpoint: cfEndpoint,
		APIKey:     apiKey,
		Model:      model,
	}
}

// Name returns the name of the provider
func (p *CloudflareOpenAIProvider) Name() string {
	return "cloudflare_openai"
}

// GetChatCompletions get chat completions for ai service
func (p *CloudflareOpenAIProvider) GetChatCompletions(userInstruction string, baseSystemMessage string, chainMessage ai.ChainMessage, md metadata.M, withTool bool) (*ai.InvokeResponse, error) {
	reqBody := openai.ReqBody{Model: p.Model}

	url := fmt.Sprintf("%s/openai/chat/completions", p.CfEndpoint)

	res, err := openai.ChatCompletion(
		url,
		"Authorization",
		fmt.Sprintf("Bearer %s", p.APIKey),
		reqBody,
		baseSystemMessage,
		userInstruction,
		chainMessage,
		md,
		withTool,
	)

	return res, err
}
