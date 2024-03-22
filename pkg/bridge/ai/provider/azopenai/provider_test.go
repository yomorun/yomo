package azopenai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
	provider := &AzureOpenAIProvider{}

	name := provider.Name()

	assert.Equal(t, "azopenai", name)
}
