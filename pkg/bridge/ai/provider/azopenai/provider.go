// Package azopenai is used to provide the Azure OpenAI service
package azopenai

import (
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/openai"
)

// AzureOpenAIProvider is the provider for Azure OpenAI
type AzureOpenAIProvider struct {
	APIKey       string
	APIEndpoint  string
	DeploymentID string
	APIVersion   string
}

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
	}
}

// Name returns the name of the provider
func (p *AzureOpenAIProvider) Name() string {
	return "azopenai"
}

// GetChatCompletions get chat completions for ai service
func (p *AzureOpenAIProvider) GetChatCompletions(userInstruction string, md metadata.M) (*ai.InvokeResponse, error) {
	// messages
	systemInstruction := `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

	reqBody := openai.ReqBody{}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", p.APIEndpoint, p.DeploymentID, p.APIVersion)
	res, err := openai.ChatCompletion(url, "api-key", p.APIKey, reqBody, systemInstruction, userInstruction, nil, md)

	if err != nil {
		return nil, err
	}

	return res, nil
}
