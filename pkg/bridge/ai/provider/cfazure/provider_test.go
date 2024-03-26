package cfazure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/mock_client"
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
	client := &mock_client.MockOpenAIClient{}

	provider := &Provider{
		CfEndpoint:   "https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test",
		APIKey:       "test",
		Resource:     "test",
		DeploymentID: "test",
		APIVersion:   "test-version",
		client:       client,
	}

	provider.GetChatCompletions("user", "system", ai.ChainMessage{}, nil, false)

	assert.Equal(t, "user", client.UserInstruction)
	assert.Equal(t, "system", client.BaseSystemMessage)
	assert.Equal(t, false, client.IfWithTool)
}
