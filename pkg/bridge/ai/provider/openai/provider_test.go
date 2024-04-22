package openai

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
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

	_, err := provider.GetChatCompletions(req, nil)

	wantErr := "Post \"https://api.openai.com/v1/chat/completions\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"
	assert.Equal(t, wantErr, err.Error())

	_, err = provider.GetChatCompletionsStream(req, nil)
	assert.Equal(t, wantErr, err.Error())
}
