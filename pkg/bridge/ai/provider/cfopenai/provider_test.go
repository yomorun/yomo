package cfopenai

import (
	"os"
	"testing"
	
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
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

	t.Run("without cfEndpoint", func(t *testing.T) {
		if os.Getenv("CI") != "true" {
			t.Skip("Skipping testing in CI environment")
		}

		assert.Panics(t, func() {
			NewProvider("", "test_api_key", "test_model")
		})
	})
}

func TestChatCompletions(t *testing.T) {
	provider := NewProvider("test_endpoint", "test_api_key", "test_model")

	resp, err := provider.GetChatCompletions("test_instruction", "test_base", ai.ChainMessage{}, metadata.M{}, false)

	assert.NotNil(t, resp)
	assert.Errorf(t, err, "no_function_call")
}
