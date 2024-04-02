// Package azopenai is used to provide the Azure OpenAI service
package azopenai

import (
	"context"
	"fmt"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
)

// Provider is the provider for Azure OpenAI
type Provider struct {
	APIKey       string
	APIEndpoint  string
	DeploymentID string
	APIVersion   string
	client       oai.OpenAIRequester
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
	return &Provider{
		APIKey:       apiKey,
		APIEndpoint:  apiEndpoint,
		DeploymentID: deploymentID,
		APIVersion:   apiVersion,
		client:       &oai.OpenAIClient{},
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "azopenai"
}

// GetChatCompletions get chat completions for ai service
// func (p *Provider) GetChatCompletions(userInstruction string, baseSystemMessage string, chainMessage ai.ChainMessage, md metadata.M, withTool bool) (*ai.InvokeResponse, error) {
func (p *Provider) GetChatCompletions(req *ai.ChatCompletionRequest) (*ai.ChatCompletionResponse, error) {
	// reqBody := oai.ReqBody{}
	// endpoint
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", p.APIEndpoint, p.DeploymentID, p.APIVersion)
	// res, err := p.client.ChatCompletion(url, "api-key", p.APIKey, reqBody, baseSystemMessage, userInstruction, chainMessage, md, withTool)
	res, err := p.client.ChatCompletions(
		context.Background(),
		url,
		"api-key",
		p.APIKey,
		req,
	)
	if err != nil {
		return nil, err
	}

	return res, nil
}
