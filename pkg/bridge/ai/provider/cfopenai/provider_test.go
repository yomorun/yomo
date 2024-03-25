package cfopenai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloudflareOpenAIProvider_Name(t *testing.T) {
	provider := &CloudflareOpenAIProvider{}

	name := provider.Name()

	assert.Equal(t, "cloudflare_openai", name)
}

func TestNewProvider(t *testing.T) {
	t.Run("with parameters", func(t *testing.T) {
		provider := NewProvider("test_endpoint", "test_api_key", "test_model")

		assert.Equal(t, "test_endpoint", provider.CfEndpoint)
		assert.Equal(t, "test_api_key", provider.APIKey)
		assert.Equal(t, "test_model", provider.Model)
	})

	t.Run("with environment variables", func(t *testing.T) {
		os.Setenv("OPENAI_API_KEY", "env_api_key")
		os.Setenv("OPENAI_MODEL", "env_model")

		provider := NewProvider("test_endpoint", "", "")

		assert.Equal(t, "test_endpoint", provider.CfEndpoint)
		assert.Equal(t, "env_api_key", provider.APIKey)
		assert.Equal(t, "env_model", provider.Model)

		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("OPENAI_MODEL")
	})
}