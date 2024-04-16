package cfopenai

import (
	"os"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
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
	config := newConfig("test", "https://faker.gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test")
	client := openai.NewClientWithConfig(config)

	provider := &Provider{
		CfEndpoint: "https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test",
		APIKey:     "test",
		Model:      "test",
		client:     client,
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

	wantErr := "Post \"https://faker.gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test/openai/chat/completions\": dial tcp: lookup faker.gateway.ai.cloudflare.com: no such host"
	assert.Equal(t, wantErr, err.Error())

	_, err = provider.GetChatCompletionsStream(req)
	assert.Equal(t, wantErr, err.Error())
}
