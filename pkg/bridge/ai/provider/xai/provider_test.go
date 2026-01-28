package xai

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/go-openai"
)

func TestXAIProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "xai", name)
}

func TestXAIProvider_GetChatCompletions(t *testing.T) {
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
		Model:    "groq-beta",
	}

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second)
	defer cancel()

	_, err := provider.GetChatCompletions(ctx, req, nil)
	assert.Error(t, err)
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(ctx, req, nil)
	assert.Error(t, err)
	t.Log(err)

	req = openai.ChatCompletionRequest{
		Messages: msgs,
	}

	_, err = provider.GetChatCompletions(ctx, req, nil)
	assert.Error(t, err)
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(ctx, req, nil)
	assert.Error(t, err)
	t.Log(err)
}
