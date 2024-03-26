package cfopenai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/mock_client"
)

func TestCloudflareOpenAIProvider_Name(t *testing.T) {
	provider := &Provider{}

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

func TestCloudflareOpenAIProvider_GetChatCompletions(t *testing.T) {
	client := &mock_client.MockOpenAIClient{}

	provider := &Provider{
		CfEndpoint: "https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test",
		APIKey:     "test",
		Model:      "test",
		client:     client,
	}

	provider.GetChatCompletions("user", "system", ai.ChainMessage{}, nil, false)

	assert.Equal(t, "test", client.BaseRequestbody.Model)
	assert.Equal(t, "user", client.UserInstruction)
	assert.Equal(t, "system", client.BaseSystemMessage)
	assert.Equal(t, false, client.IfWithTool)
}
