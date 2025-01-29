package cerebras

import (
	"context"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestCerebrasProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "deepseek", name)
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
		Model:    "deepseek-chat",
	}

	_, err := provider.GetChatCompletions(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)
}
