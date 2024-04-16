// Package azopenai is used to provide the Azure OpenAI service
package azopenai

import (
	"context"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"

	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// Provider is the provider for Azure OpenAI
type Provider struct {
	APIKey       string
	APIEndpoint  string
	DeploymentID string
	APIVersion   string
	client       *openai.Client
}

var _ bridgeai.LLMProvider = &Provider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(apiKey string, apiEndpoint string, deploymentID string, apiVersion string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("AZURE_OPENAI_API_KEY")
	}
	if apiEndpoint == "" {
		apiEndpoint = os.Getenv("AZURE_OPENAI_API_ENDPOINT")
	}
	if deploymentID == "" {
		deploymentID = os.Getenv("AZURE_OPENAI_DEPLOYMENT_ID")
	}
	if apiVersion == "" {
		apiVersion = os.Getenv("AZURE_OPENAI_API_VERSION")
	}

	config := newConfig(apiKey, apiEndpoint, deploymentID, apiVersion)

	client := openai.NewClientWithConfig(config)

	return &Provider{
		APIKey:       apiKey,
		APIEndpoint:  apiEndpoint,
		DeploymentID: deploymentID,
		APIVersion:   apiVersion,
		client:       client,
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "azopenai"
}

func newConfig(apiKey string, apiEndpoint string, deploymentID string, apiVersion string) openai.ClientConfig {
	config := openai.DefaultAzureConfig(apiKey, apiEndpoint)
	config.AzureModelMapperFunc = func(model string) string { return deploymentID }
	config.APIVersion = apiVersion

	return config
}

// GetChatCompletions get chat completions for ai service
func (p *Provider) GetChatCompletions(req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return p.client.CreateChatCompletion(context.Background(), req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return p.client.CreateChatCompletionStream(context.Background(), req)
}
