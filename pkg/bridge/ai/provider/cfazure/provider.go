// Package cfazure is used to provide the Azure OpenAI service
package cfazure

import (
	"context"
	"fmt"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
)

// Provider is the provider for Azure OpenAI
type Provider struct {
	APIKey       string
	Resource     string
	DeploymentID string
	APIVersion   string
	CfEndpoint   string
	client       oai.OpenAIRequester
}

// check if implements ai.Provider
var _ bridgeai.LLMProvider = &Provider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(cfEndpoint string, apiKey string, resource string, deploymentID string, apiVersion string) *Provider {
	if cfEndpoint == "" || apiKey == "" || resource == "" || deploymentID == "" {
		ylog.Error("parameters are required", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID)
		return nil
	}
	ylog.Debug("CloudflareAzureProvider", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID, "apiVersion", apiVersion)
	return &Provider{
		CfEndpoint:   cfEndpoint,   // https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test
		APIKey:       apiKey,       // azure api key
		Resource:     resource,     // azure resource
		DeploymentID: deploymentID, // azure deployment id
		APIVersion:   apiVersion,   // azure api version
		client:       &oai.OpenAIClient{},
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "cloudflare_azure"
}

// GetChatCompletions get chat completions for ai service
func (p *Provider) GetChatCompletions(req *ai.ChatCompletionRequest) (*ai.ChatCompletionResponse, error) {
	url := fmt.Sprintf("%s/azure-openai/%s/%s/chat/completions?api-version=%s", p.CfEndpoint, p.Resource, p.DeploymentID, p.APIVersion)

	res, err := p.client.ChatCompletions(context.Background(), url, "api-key", p.APIKey, req)

	return res, err
}
