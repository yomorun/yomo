// Package cfazure is used to provide the Azure OpenAI service
package cfazure

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

// CloudflareAzureProvider is the provider for Azure OpenAI
type CloudflareAzureProvider struct {
	APIKey       string
	Resource     string
	DeploymentID string
	APIVersion   string
	CfEndpoint   string
}

// check if implements ai.Provider
var _ bridgeai.LLMProvider = &CloudflareAzureProvider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(cfEndpoint string, apiKey string, resource string, deploymentID string, apiVersion string) *CloudflareAzureProvider {
	if cfEndpoint == "" || apiKey == "" || resource == "" || deploymentID == "" || apiVersion == "" {
		ylog.Error("parameters are required", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID, "apiVersion", apiVersion)
		os.Exit(-1)
		return nil
	}
	ylog.Debug("CloudflareAzureProvider", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID, "apiVersion", apiVersion)
	return &CloudflareAzureProvider{
		CfEndpoint:   cfEndpoint,   // https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test
		APIKey:       apiKey,       // azure api key
		Resource:     resource,     // azure resource
		DeploymentID: deploymentID, // azure deployment id
		APIVersion:   apiVersion,   // azure api version
	}
}

// Name returns the name of the provider
func (p *CloudflareAzureProvider) Name() string {
	return "cloudflare_azure"
}

// GetChatCompletions get chat completions for ai service
func (p *CloudflareAzureProvider) GetChatCompletions(userInstruction string, baseSystemMessage string, previousToolCalls []*ai.ToolCall, md metadata.M) (*ai.InvokeResponse, error) {
	// messages
	// userDefinedBaseSystemMessage := `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

	reqBody := openai.ReqBody{}

	url := fmt.Sprintf("%s/azure-openai/%s/%s/chat/completions?api-version=%s", p.CfEndpoint, p.Resource, p.DeploymentID, p.APIVersion)

	res, err := openai.ChatCompletion(url, "api-key", p.APIKey, reqBody, baseSystemMessage, userInstruction, previousToolCalls, md)

	return res, err
}
