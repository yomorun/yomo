// Package cfazure is used to provide the Azure OpenAI service
package cfazure

import (

	// automatically load .env file
	"context"
	"fmt"

	_ "github.com/joho/godotenv/autoload"
	openai "github.com/sashabaranov/go-openai"

	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// Provider is the provider for Azure OpenAI
type Provider struct {
	APIKey       string
	Resource     string
	DeploymentID string
	APIVersion   string
	CfEndpoint   string
	client       *openai.Client
}

// check if implements ai.Provider
var _ bridgeai.LLMProvider = &Provider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(cfEndpoint string, apiKey string, resource string, deploymentID string, apiVersion string) *Provider {
	if cfEndpoint == "" || apiKey == "" || resource == "" || deploymentID == "" {
		ylog.Error("parameters are required", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID)
		return nil
	}

	config := newConfig(cfEndpoint, apiKey, resource, deploymentID, apiVersion)

	client := openai.NewClientWithConfig(config)

	ylog.Debug("CloudflareAzureProvider", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID, "apiVersion", apiVersion)
	return &Provider{
		CfEndpoint:   cfEndpoint,   // https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test
		APIKey:       apiKey,       // azure api key
		Resource:     resource,     // azure resource
		DeploymentID: deploymentID, // azure deployment id
		APIVersion:   apiVersion,   // azure api version
		client:       client,
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "cloudflare_azure"
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return p.client.CreateChatCompletion(context.Background(), req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return p.client.CreateChatCompletionStream(context.Background(), req)
}

func newConfig(cfEndpoint string, apiKey string, resource string, deploymentID string, apiVersion string) openai.ClientConfig {
	baseUrl := fmt.Sprintf("%s/azure-openai/%s/%s", cfEndpoint, resource, deploymentID)

	config := openai.DefaultAzureConfig(apiKey, baseUrl)
	config.APIType = openai.APITypeCloudflareAzure
	config.APIVersion = apiVersion

	return config
}
