// Package oai provides the ability to call OpenAI api
package oai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

type OpenAIClient struct{}

var _ ILLMClient = &OpenAIClient{}

func (c *OpenAIClient) ChatCompletion(apiEndpoint string, authHeaderKey string, authHeaderValue string, baseRequestbody ReqBody, baseSystemMessage string, userInstruction string, chainMessage ai.ChainMessage, md metadata.M, ifWithTool bool) (*ai.InvokeResponse, error) {
	// read tools attached to the metadata
	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return nil, err
	}

	// prepare tools
	toolCalls, err := c.prepareToolCalls(tcs)
	if err != nil {
		return nil, err
	}

	// prepare prompts, the prompts structure should be:
	// - system messsage
	//   - base system message (user defined)
	//   - tool's appended instruction (inject)
	// - [] history messages (inject previous tool_call response when finish_reason is 'tools_call')
	// - user instruction
	prompt := c.preparePrompt(baseSystemMessage, userInstruction, chainMessage, toolCalls, ifWithTool)

	// prepare request body
	reqBody, err := c.prepareRequestBody(prompt, ifWithTool, toolCalls, baseRequestbody)
	if err != nil {
		return nil, err
	}

	// send restful request
	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// authentication header
	req.Header.Set(authHeaderKey, authHeaderValue)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ylog.Debug(">response", "body", string(respBody))

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ai response status code is %d", resp.StatusCode)
	}

	// handle response
	result, err := c.handleResponse(respBody, tcs)
	return result, err
}

func (c *OpenAIClient) prepareToolCalls(tcs map[uint32]ai.ToolCall) ([]ai.ToolCall, error) {
	// prepare tools
	toolCalls := make([]ai.ToolCall, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = tc
		idx++
	}
	return toolCalls, nil
}

func (c *OpenAIClient) preparePrompt(baseSystemMessage string, userInstruction string, chainMessage ai.ChainMessage, toolCalls []ai.ToolCall, ifWithTool bool) []ChatCompletionMessage {
	systemInstructions := []string{"## Instructions\n"}

	// only append if there are tool calls
	if ifWithTool {
		for _, tc := range toolCalls {
			systemInstructions = append(systemInstructions, "- ")
			systemInstructions = append(systemInstructions, tc.Function.Description)
			systemInstructions = append(systemInstructions, "\n")
		}
		systemInstructions = append(systemInstructions, "\n")
	}

	SystemPrompt := fmt.Sprintf("%s\n\n%s", baseSystemMessage, strings.Join(systemInstructions, ""))

	messages := []ChatCompletionMessage{}

	// 1. system message
	messages = append(messages, ChatCompletionMessage{Role: "system", Content: SystemPrompt})

	// 2. previous tool calls
	// Ref: Tool Message Object in Messsages
	// https://platform.openai.com/docs/guides/function-calling
	// https://platform.openai.com/docs/api-reference/chat/create#chat-create-messages

	if chainMessage.PreceedingAssistantMessage != nil {
		// 2.1 assistant message
		// try convert type of chainMessage.PreceedingAssistantMessage to type ChatCompletionMessage
		assistantMessage, ok := chainMessage.PreceedingAssistantMessage.(ChatCompletionMessage)
		if ok {
			ylog.Debug("======== add assistantMessage", "am", fmt.Sprintf("%+v", assistantMessage))
			messages = append(messages, assistantMessage)
		}

		// 2.2 tool message
		for _, tool := range chainMessage.ToolMessages {
			tm := ChatCompletionMessage{
				Role:       "tool",
				Content:    tool.Content,
				ToolCallID: tool.ToolCallId,
			}
			ylog.Debug("======== add toolMessage", "tm", fmt.Sprintf("%+v", tm))
			messages = append(messages, tm)
		}
	}

	// 3. user instruction
	messages = append(messages, ChatCompletionMessage{Role: "user", Content: userInstruction})

	return messages
}

func (c *OpenAIClient) prepareRequestBody(prompt []ChatCompletionMessage, ifWithTool bool, toolCalls []ai.ToolCall, baseRequestbody ReqBody) ([]byte, error) {
	baseRequestbody.Messages = prompt

	if ifWithTool && len(toolCalls) > 0 {
		baseRequestbody.Tools = toolCalls
	}

	jsonBody, err := json.Marshal(baseRequestbody)
	ylog.Debug("< request", "body", string(jsonBody))
	return jsonBody, err
}

func (c *OpenAIClient) handleResponse(respBody []byte, tcs map[uint32]ai.ToolCall) (*ai.InvokeResponse, error) {
	var respBodyStruct RespBody
	err := json.Unmarshal(respBody, &respBodyStruct)
	if err != nil {
		return nil, err
	}

	// the first choice is the result
	choice := respBodyStruct.Choices[0]

	ylog.Debug(">>finish_reason", "reason", choice.FinishReason)

	responseMessage := respBodyStruct.Choices[0].Message
	calls := responseMessage.ToolCalls
	content := responseMessage.Content

	ylog.Debug("--response calls", "calls", len(calls))

	result := &ai.InvokeResponse{}

	// finish reason
	result.FinishReason = choice.FinishReason
	result.Content = content

	// record usage
	result.TokenUsage = ai.TokenUsage{
		PromptTokens:     respBodyStruct.Usage.PromptTokens,
		CompletionTokens: respBodyStruct.Usage.CompletionTokens,
	}
	ylog.Debug("++ llm result", "token_usage", fmt.Sprintf("%v", result.TokenUsage), "finish_reason", result.FinishReason)

	// if llm said no function call, we should return the result
	if result.FinishReason == "stop" {
		return result, nil
	}

	if result.FinishReason == "tool_calls" {
		// assistant message
		result.AssistantMessage = responseMessage
	}

	if len(calls) == 0 {
		return result, errors.New("finish_reason is tool_calls, but no tool calls found")
	}

	// functions may be more than one
	for _, call := range calls {
		for tag, tc := range tcs {
			if tc.Equal(call) {
				// use toolCalls because tool_id is required in the following llm request
				if result.ToolCalls == nil {
					result.ToolCalls = make(map[uint32][]*ai.ToolCall)
				}
				// create a new variable to hold the current call
				currentCall := call
				result.ToolCalls[tag] = append(result.ToolCalls[tag], currentCall)
			}
		}
	}

	return result, nil
}
