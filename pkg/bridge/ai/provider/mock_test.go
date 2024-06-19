package provider

import (
	"context"
	"io"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestMockProviderRequest(t *testing.T) {
	provider, err := NewMock("mock",
		MockChatCompletionResponse(data),
		MockChatCompletionStreamResponse(streamData))
	assert.NoError(t, err)

	reqs := []openai.ChatCompletionRequest{
		{Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "hi, llm bridge"}}},
		{Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "hi, yomo"}}},
	}

	provider.GetChatCompletions(context.TODO(), reqs[0])
	provider.GetChatCompletionsStream(context.TODO(), reqs[1])

	assert.Equal(t, reqs, provider.RequestRecords())
}

func TestMockProvider(t *testing.T) {
	provider, err := NewMock("mock",
		MockChatCompletionResponse(data),
		MockChatCompletionStreamResponse(streamData))
	assert.NoError(t, err)

	t.Run("Name()", func(t *testing.T) {
		assert.Equal(t, "mock", provider.Name())
	})

	t.Run("GetChatCompletions()", func(t *testing.T) {
		resp, err := provider.GetChatCompletions(context.TODO(), openai.ChatCompletionRequest{Model: "gpt-4o-2024-05-13"})
		assert.NoError(t, err)
		assert.Equal(t, "chatcmpl-9b9wyqGfbZHi0pPDfYgYKXAV1obkm", resp.ID)
		assert.Equal(t, "gpt-4o-2024-05-13", resp.Model)
		assert.Equal(t, "One plus one equals two.", resp.Choices[0].Message.Content)
	})

	t.Run("GetChatCompletionsStream()", func(t *testing.T) {
		recver, err := provider.GetChatCompletionsStream(context.TODO(), openai.ChatCompletionRequest{})
		assert.NoError(t, err)

		for {
			resp, err := recver.Recv()
			if err == io.EOF {
				break
			}
			assert.NoError(t, err)
			assert.Equal(t, "chatcmpl-9b2Ah9pTmqiVMkwZEPBLqJpLzFzGG", resp.ID)
			assert.Equal(t, "gpt-4o-2024-05-13", resp.Model)
		}
	})
}

var data = `{
	"id": "chatcmpl-9b9wyqGfbZHi0pPDfYgYKXAV1obkm",
	"object": "chat.completion",
	"created": 1718643412,
	"model": "gpt-4o-2024-05-13",
	"choices": [
	  {
		"index": 0,
		"message": {
		  "role": "assistant",
		  "content": "One plus one equals two."
		},
		"logprobs": null,
		"finish_reason": "stop"
	  }
	],
	"usage": {
	  "prompt_tokens": 13,
	  "completion_tokens": 6,
	  "total_tokens": 19
	},
	"system_fingerprint": "fp_319be4768e"
  }`

var streamData = `data: {"id":"chatcmpl-9b2Ah9pTmqiVMkwZEPBLqJpLzFzGG","object":"chat.completion.chunk","created":1718613511,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_aa87380ac5","choices":[{"index":0,"delta":{"content":" accurate"},"logprobs":null,"finish_reason":null}]}
  
  data: {"id":"chatcmpl-9b2Ah9pTmqiVMkwZEPBLqJpLzFzGG","object":"chat.completion.chunk","created":1718613511,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_aa87380ac5","choices":[{"index":0,"delta":{"content":" conversion"},"logprobs":null,"finish_reason":null}]}
  
  data: {"id":"chatcmpl-9b2Ah9pTmqiVMkwZEPBLqJpLzFzGG","object":"chat.completion.chunk","created":1718613511,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_aa87380ac5","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}
  
  data: {"id":"chatcmpl-9b2Ah9pTmqiVMkwZEPBLqJpLzFzGG","object":"chat.completion.chunk","created":1718613511,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_aa87380ac5","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}
  
  data: [DONE]`
