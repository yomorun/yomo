package githubmodels

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	openai "github.com/yomorun/go-openai"
)

func TestGithubModelsProvider_Name(t *testing.T) {
	provider := &Provider{}

	name := provider.Name()

	assert.Equal(t, "githubmodels", name)
}

func TestNewProvider(t *testing.T) {
	t.Run("with parameters", func(t *testing.T) {
		provider := NewProvider("test_api_key", "test_model")

		assert.Equal(t, "test_api_key", provider.APIKey)
		assert.Equal(t, "test_model", provider.Model)
	})

	t.Run("with environment variables", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "env_api_key")

		provider := NewProvider("", "test_model")

		assert.Equal(t, "env_api_key", provider.APIKey)
		assert.Equal(t, "test_model", provider.Model)

		os.Unsetenv("GITHUB_TOKEN")
	})
}

func TestGithubModelsProvider_GetChatCompletions(t *testing.T) {
	config := openai.DefaultConfig("test_api_key")
	config.BaseURL = "https://models.inference.ai.azure.com"
	client := openai.NewClientWithConfig(config)

	provider := &Provider{
		APIKey: "test_api_key",
		Model:  "test_model",
		client: client,
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

	_, err := provider.GetChatCompletions(context.TODO(), req, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestGithubModelsProvider_GetChatCompletionsStream(t *testing.T) {
	config := openai.DefaultConfig("test_api_key")
	config.BaseURL = "https://models.inference.ai.azure.com"
	client := openai.NewClientWithConfig(config)

	provider := &Provider{
		APIKey: "test_api_key",
		Model:  "test_model",
		client: client,
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

	_, err := provider.GetChatCompletionsStream(context.TODO(), req, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
