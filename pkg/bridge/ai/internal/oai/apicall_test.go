package oai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
)

var tcs = map[uint32]ai.ToolCall{
	1: {Function: &ai.FunctionDefinition{Name: "function1"}},
	2: {Function: &ai.FunctionDefinition{Name: "function1"}},
}

var tcArray = []ai.ToolCall{
	{Function: &ai.FunctionDefinition{Name: "function1"}},
	{Function: &ai.FunctionDefinition{Name: "function1"}},
}

func TestOpenAIClient_prepareToolCalls(t *testing.T) {
	client := &OpenAIClient{}
	toolCalls, err := client.prepareToolCalls(tcs)

	assert.Nil(t, err)
	assert.Equal(t, len(tcs), len(toolCalls))

	for i, tc := range toolCalls {
		assert.Equal(t, tcs[uint32(i+1)].Function.Name, tc.Function.Name)
	}
}

func TestOpenAIClient_preparePrompt(t *testing.T) {
	client := &OpenAIClient{}

	baseSystemMessage := "base system message"
	userInstruction := "user instruction"
	chainMessage := ai.ChainMessage{
		PreceedingAssistantMessage: ChatCompletionMessage{Role: "assistant", Content: "assistant message"},
		ToolMessages: []ai.ToolMessage{
			{Content: "tool message 1", ToolCallId: "id 1"},
			{Content: "tool message 2", ToolCallId: "id 2"},
		},
	}
	toolCalls := tcArray
	ifWithTool := true

	messages := client.preparePrompt(baseSystemMessage, userInstruction, chainMessage, toolCalls, ifWithTool)

	assert.Equal(t, 5, len(messages))

	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, baseSystemMessage)

	assert.Equal(t, "assistant", messages[1].Role)
	assert.Equal(t, "assistant message", messages[1].Content)

	assert.Equal(t, "tool", messages[2].Role)
	assert.Equal(t, chainMessage.ToolMessages[0].Content, messages[2].Content)
	assert.Equal(t, chainMessage.ToolMessages[0].ToolCallId, messages[2].ToolCallID)
	assert.Equal(t, chainMessage.ToolMessages[1].Content, messages[3].Content)
	assert.Equal(t, chainMessage.ToolMessages[1].ToolCallId, messages[3].ToolCallID)

	assert.Equal(t, "user", messages[4].Role)
	assert.Equal(t, userInstruction, messages[4].Content)
}

func TestOpenAIClient_prepareRequestBody(t *testing.T) {
	client := &OpenAIClient{}

	prompt := []ChatCompletionMessage{
		{Role: "user", Content: "user message"},
		{Role: "assistant", Content: "assistant message"},
	}
	ifWithTool := true
	toolCalls := tcArray
	baseRequestBody := ReqBody{}

	jsonBody, err := client.prepareRequestBody(prompt, ifWithTool, toolCalls, baseRequestBody)
	assert.Nil(t, err)

	var requestBody ReqBody
	err = json.Unmarshal(jsonBody, &requestBody)

	assert.Nil(t, err)
	assert.Equal(t, len(prompt), len(requestBody.Messages))
	assert.Equal(t, len(toolCalls), len(requestBody.Tools))
}

func TestOpenAIClient_handleResponse(t *testing.T) {
	client := &OpenAIClient{}
	respBody := `{
		"id": "chatcmpl-96qRSmbmYqDQQaZPVuUATQ8A2xY3s",
		"object": "chat.completion",
		"created": 1711418582,
		"model": "gpt-4-1106-preview",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [
						{
							"id": "call_gYNrFEb6tuT764gFdwwRWVl2",
							"type": "function",
							"function": {
								"name": "fn-timezone-converter",
								"arguments": "{\"sourceTimezone\": \"America/Los_Angeles\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-03-25 17:00:00\"}"
							}
						},
						{
							"id": "call_ZppTYd1MaDkTcaORDFiNlRFg",
							"type": "function",
							"function": {
								"name": "fn-timezone-converter",
								"arguments": "{\"sourceTimezone\": \"America/New_York\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-03-25 17:00:00\"}"
							}
						}
					]
				},
				"logprobs": null,
				"finish_reason": "tool_calls"
			}
		],
		"usage": {
			"prompt_tokens": 422,
			"completion_tokens": 157,
			"total_tokens": 579
		},
		"system_fingerprint": "fp_123d5a9f90"
	}
	`

	toolCalls := map[uint32]ai.ToolCall{
		100: {
			Type:     "function",
			Function: &ai.FunctionDefinition{Name: "fn-timezone-converter"}},
	}

	resp, err := client.handleResponse([]byte(respBody), toolCalls)

	assert.Nil(t, err)
	assert.Equal(t, "tool_calls", resp.FinishReason)
	assert.Empty(t, resp.Content)
	assert.Equal(t, 1, len(resp.ToolCalls))
	assert.Equal(t, 2, len(resp.ToolCalls[100]))
	assert.Equal(t, "call_gYNrFEb6tuT764gFdwwRWVl2", resp.ToolCalls[100][0].ID)
	assert.Equal(t, "call_ZppTYd1MaDkTcaORDFiNlRFg", resp.ToolCalls[100][1].ID)
	assert.Equal(t, "{\"sourceTimezone\": \"America/Los_Angeles\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-03-25 17:00:00\"}", resp.ToolCalls[100][0].Function.Arguments)
	assert.Equal(t, "{\"sourceTimezone\": \"America/New_York\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-03-25 17:00:00\"}", resp.ToolCalls[100][1].Function.Arguments)
}
