// Package azopenai is used to provide the Azure OpenAI service
package azopenai

import (
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
)

// AzureOpenAIProvider is the provider for Azure OpenAI
type AzureOpenAIProvider struct {
	APIKey       string
	APIEndpoint  string
	DeploymentID string
	APIVersion   string
	client       oai.ILLMClient
}

var _ bridgeai.LLMProvider = &AzureOpenAIProvider{}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(apiKey string, apiEndpoint string, deploymentID string, apiVersion string) *AzureOpenAIProvider {
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
	return &AzureOpenAIProvider{
		APIKey:       apiKey,
		APIEndpoint:  apiEndpoint,
		DeploymentID: deploymentID,
		APIVersion:   apiVersion,
		client:       &oai.OpenAIClient{},
	}
}

// Name returns the name of the provider
func (p *AzureOpenAIProvider) Name() string {
	return "azopenai"
}

// GetChatCompletions get chat completions for ai service
func (p *AzureOpenAIProvider) GetChatCompletions(userInstruction string, baseSystemMessage string, chainMessage ai.ChainMessage, md metadata.M, withTool bool) (*ai.InvokeResponse, error) {
	reqBody := oai.ReqBody{}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", p.APIEndpoint, p.DeploymentID, p.APIVersion)
	res, err := p.client.ChatCompletion(url, "api-key", p.APIKey, reqBody, baseSystemMessage, userInstruction, chainMessage, md, withTool)

	if err != nil {
		return nil, err
	}

	return res, nil
}
