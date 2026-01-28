package vllm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/go-openai"
)

func TestVLlmProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "vllm", name)
}

func TestVLlmProvider_GetChatCompletions(t *testing.T) {
	provider := NewProvider("", "", "")
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
		Model:    "meta-llama/Llama-3.2-7B-Instruct",
	}

	_, err := provider.GetChatCompletions(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)
}

func TestVLlmProvider_GetChatCompletionsWithoutModel(t *testing.T) {
	provider := NewProvider("", "", "")
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
	t.Log(err)

	_, err = provider.GetChatCompletionsStream(context.TODO(), req, nil)
	assert.Error(t, err)
	t.Log(err)
}
