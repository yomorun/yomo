package openai

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/go-openai"
)

func TestNewProvider(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("OPENAI_API_KEY", "test_api_key")
	os.Setenv("OPENAI_MODEL", "test_model")

	provider := NewProvider("", "")

	assert.Equal(t, "test_api_key", provider.APIKey)
	assert.Equal(t, "test_model", provider.Model)
}

func TestOpenAIProvider_Name(t *testing.T) {
	provider := &Provider{}

	name := provider.Name()

	assert.Equal(t, "openai", name)
}

func TestCloudflareOpenAIProvider_GetChatCompletions(t *testing.T) {
	config := openai.DefaultConfig("test-api-key")
	config.HTTPClient = &http.Client{Timeout: time.Millisecond}
	client := openai.NewClientWithConfig(config)

	provider := &Provider{
		APIKey: "test",
		Model:  "test",
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
		Model:    "gp-3.5-turbo",
		Messages: msgs,
	}

	_, err := provider.GetChatCompletions(context.TODO(), req, nil)
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(context.TODO(), req, nil)
	t.Log(err)
}
