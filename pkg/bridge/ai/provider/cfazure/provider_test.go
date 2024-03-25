package cfazure

import (
	"testing"

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
	provider := &CloudflareAzureProvider{}
	name := provider.Name()
	assert.Equal(t, "cloudflare_azure", name)
}
