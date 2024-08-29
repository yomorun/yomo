package cerebras

import (
	"context"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestNewProvider(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("CEREBRAS_API_KEY", "test_api_key")
	os.Setenv("CEREBRAS_MODEL", "llama3.1-70b")

	provider := NewProvider("", "")
	assert.Equal(t, "test_api_key", provider.APIKey)
	assert.Equal(t, "llama3.1-70b", provider.Model)
}

func TestCerebrasProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "cerebras", name)
}

func TestCerebrasProvider_GetChatCompletions(t *testing.T) {
	provider := NewProvider("", "")
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
		Model:    "llama3.1-8b",
	}

	_, err := provider.GetChatCompletions(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)
}
