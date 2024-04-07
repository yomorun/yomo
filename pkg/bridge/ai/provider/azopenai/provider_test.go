package azopenai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/mock_client"
)

func TestNewProvider(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("AZURE_OPENAI_API_KEY", "test_api_key")
	os.Setenv("AZURE_OPENAI_API_ENDPOINT", "test_api_endpoint")
	os.Setenv("AZURE_OPENAI_DEPLOYMENT_ID", "test_deployment_id")
	os.Setenv("AZURE_OPENAI_API_VERSION", "test_api_version")

	provider := NewProvider("", "", "", "")

	assert.Equal(t, "test_api_key", provider.APIKey)
	assert.Equal(t, "test_api_endpoint", provider.APIEndpoint)
	assert.Equal(t, "test_deployment_id", provider.DeploymentID)
	assert.Equal(t, "test_api_version", provider.APIVersion)
}

func TestAzureOpenAIProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "azopenai", name)
}

func TestAzureOpenAIProvider_GetChatCompletions(t *testing.T) {
	client := &mock_client.MockOpenAIClient{}

	provider := &Provider{
		APIKey:       "test",
		APIEndpoint:  "https://yomo.openai.azure.com",
		DeploymentID: "test",
		APIVersion:   "test-version",
		client:       client,
	}
	msgs := []ai.ChatCompletionMessage{
		{
			Role:    "user",
			Content: "hello",
		},
		{
			Role:    "system",
			Content: "I'm a bot",
		},
	}
	req := &ai.ChatCompletionRequest{
		Model:    "gp-3.5-turbo",
		Messages: msgs,
	}
	_, err := provider.GetChatCompletions(req)

	assert.Equal(t, nil, err)
}
