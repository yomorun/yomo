package cfazure

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestNewProvider(t *testing.T) {
	// Test with empty parameters
	provider := NewProvider("", "", "", "", "api_version_can_be_empty")
	assert.Nil(t, provider)

	// Test with valid parameters
	cfEndpoint := "https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test"
	apiKey := "azure api key"
	resource := "azure resource"
	deploymentID := "azure deployment id"
	apiVersion := "azure api version"
	provider = NewProvider(cfEndpoint, apiKey, resource, deploymentID, apiVersion)
	assert.NotNil(t, provider)
	assert.Equal(t, cfEndpoint, provider.CfEndpoint)
	assert.Equal(t, apiKey, provider.APIKey)
	assert.Equal(t, resource, provider.Resource)
	assert.Equal(t, deploymentID, provider.DeploymentID)
	assert.Equal(t, apiVersion, provider.APIVersion)
}

func TestName(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()
	assert.Equal(t, "cloudflare_azure", name)
}

func TestCloudflareAzureProvider_GetChatCompletions(t *testing.T) {
	config := newConfig("https://facker.gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test", "test", "test", "test", "test-version")
	client := openai.NewClientWithConfig(config)

	provider := &Provider{
		CfEndpoint:   "https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test",
		APIKey:       "test",
		Resource:     "test",
		DeploymentID: "test",
		APIVersion:   "test-version",
		client:       client,
	}

	msgs := []openai.ChatCompletionMessage{
		{
			Role:    "user",
			Content: "hello",
		},
		{
			Role:    "system",
			Content: "I'm a bot",
		},
	}
	req := openai.ChatCompletionRequest{
		Messages: msgs,
	}

	_, err := provider.GetChatCompletions(req)

	wantErr := "Post \"https://facker.gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test/azure-openai/test/test/chat/completions?api-version=test-version\": dial tcp: lookup facker.gateway.ai.cloudflare.com: no such host"
	assert.Equal(t, wantErr, err.Error())

	_, err = provider.GetChatCompletionsStream(req)
	assert.Equal(t, wantErr, err.Error())
}
