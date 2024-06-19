package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/golang-lru/v2/expirable"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

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

func TestCaller(t *testing.T) {
	provider, err := provider.NewMock("mock provider", provider.MockChatCompletionResponse(data))
	if err != nil {
		t.Fatal(err)
	}
	cp.provideFunc = mockCallerProvideFunc(map[uint32]map[string]mockFunctionCall{
		1: {"call-1": {toolID: "tool-1", functionName: "function-1", resContent: "res-1"}},
		2: {"call-2": {toolID: "tool-2", functionName: "function-2", resContent: "res-2"}},
	}, provider)

	caller, err := cp.Provide("credential")
	assert.NoError(t, err)

	caller.SetSystemPrompt("system prompt")

	_, err = caller.GetInvoke(context.TODO(), "user instruction", "base system message", "transID", true)
	assert.NoError(t, err)
}

var cp = &CallerProvider{
	zipperAddr: DefaultZipperAddr,
	exFn:       DefaultExchangeMetadataFunc,
	callers:    expirable.NewLRU(CallerProviderCacheSize, func(_ string, caller *Caller) { caller.Close() }, CallerProviderCacheTTL),
}

func mockCallerProvideFunc(calls map[uint32]map[string]mockFunctionCall, p provider.LLMProvider) provideFunc {
	// register function to register
	for tag, call := range calls {
		for _, c := range call {
			register.RegisterFunction(tag, &openai.FunctionDefinition{Name: c.functionName}, uint64(tag), nil)
		}
	}

	return func(credential, _ string, provider provider.LLMProvider, _ ExchangeMetadataFunc) (*Caller, error) {
		caller := &Caller{
			credential: credential,
			provider:   p,
			md:         metadata.M{"hello": "llm bridge"},
		}

		caller.SetSystemPrompt("system prompt")
		caller.CallSyncer = &mockCallSyncer{}

		return caller, nil
	}
}

type mockFunctionCall struct {
	toolID       string
	functionName string
	resContent   string
}

type mockCallSyncer struct {
	calls map[uint32]map[string]mockFunctionCall
}

// Call implements CallSyncer.
func (m *mockCallSyncer) Call(ctx context.Context, transID string, reqID string, toolCalls map[uint32][]*openai.ToolCall) ([]openai.ChatCompletionMessage, error) {
	res := []openai.ChatCompletionMessage{}
	for tag, calls := range toolCalls {
		mcs, ok := m.calls[tag]
		if !ok {
			return nil, errors.New("call not found")
		}
		for _, call := range calls {
			mc, ok := mcs[call.ID]
			if !ok {
				return nil, errors.New("call not found")
			}
			res = append(res, openai.ChatCompletionMessage{
				ToolCallID: mc.toolID,
				Role:       openai.ChatMessageRoleTool,
				Content:    mc.resContent,
			})
		}
	}
	return res, nil
}

func (m *mockCallSyncer) Close() error { return nil }
