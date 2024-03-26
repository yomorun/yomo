// Package openai is the OpenAI llm provider
package openai

import (
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"

	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// APIEndpoint is the endpoint for OpenAI
const APIEndpoint = "https://api.openai.com/v1/chat/completions"

// OpenAIProvider is the provider for OpenAI
type OpenAIProvider struct {
	// APIKey is the API key for OpenAI
	APIKey string
	// Model is the model for OpenAI
	// eg. "gpt-3.5-turbo-1106", "gpt-4-turbo-preview", "gpt-4-vision-preview", "gpt-4"
	Model  string
	client oai.ILLMClient
}

// check if implements ai.Provider
var _ bridgeai.LLMProvider = &OpenAIProvider{}

// NewProvider creates a new OpenAIProvider
func NewProvider(apiKey string, model string) *OpenAIProvider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	ylog.Debug("new openai provider", "api_endpoint", APIEndpoint, "api_key", apiKey, "model", model)
	return &OpenAIProvider{
		APIKey: apiKey,
		Model:  model,
		client: &oai.OpenAIClient{},
	}
}

// Name returns the name of the provider
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// GetChatCompletions get chat completions for ai service
func (p *OpenAIProvider) GetChatCompletions(userInstruction string, baseSystemMessage string, chainMessage ai.ChainMessage, md metadata.M, withTool bool) (*ai.InvokeResponse, error) {
	reqBody := oai.ReqBody{Model: p.Model}

	res, err := p.client.ChatCompletion(APIEndpoint, "Authorization", fmt.Sprintf("Bearer %s", p.APIKey), reqBody, baseSystemMessage, userInstruction, chainMessage, md, withTool)
	if err != nil {
		return nil, err
	}

	return res, nil
}
