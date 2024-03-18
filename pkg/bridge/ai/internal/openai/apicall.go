package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

// GetChatCompletions get chat completions for ai service
func ChatCompletion(apiEndpoint string, authHeaderKey, authHeaderValue string, baseRequestbody ReqBody, baseSystemInstruction, userInstruction string, attachedMessages []ChatCompletionMessage, md metadata.M) (*ai.InvokeResponse, error) {
	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return nil, err
	}
	if len(tcs) == 0 {
		ylog.Error(ai.ErrNoFunctionCall.Error())
		return &ai.InvokeResponse{Content: "no toolcalls"}, ai.ErrNoFunctionCall
	}

	// prepare tools
	toolCalls := make([]ai.ToolCall, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = tc
		idx++
	}

	/*** messages should be constructed like this:
	// - system messsage
	//   - base system message (user defined)
	//   - tool's appended instruction (inject)
	// - [] history messages (inject last tool_call response when finish_reason is 'tools_call')
	// - user instruction
	***/

	// TODO: This demostates how to construct the messages
	// this should be read from the sfn description
	baseSystemInstruction += `
		
	## Instructions
	- if user asks concurrency exchange rate related questions, and the target currency is GBP (British Pound), you should ignore calling tools.
	- if user asks timezone converter related questions, extract the source time to 'timeString', extract the source timezone information and 'sourceTimezone', extract the target timezone information to 'targetTimezone'. The desired 'timeString' format is 'YYYY-MM-DD HH:MM:SS'. the 'sourceTimezone' and 'targetTimezone' are in IANA Time Zone Database identifier format. The function will convert the time from the source timezone to the target timezone and return the converted time as a string in the format 'YYYY-MM-DD HH:MM:SS'. If you are not sure about the date value of 'timeString', set date value to '1900-01-01'
	`

	messages := []ChatCompletionMessage{}

	messages = append(messages, ChatCompletionMessage{Role: "system", Content: baseSystemInstruction})
	messages = append(messages, attachedMessages...)
	messages = append(messages, ChatCompletionMessage{Role: "user", Content: userInstruction})

	baseRequestbody.Messages = messages
	baseRequestbody.Tools = toolCalls

	jsonBody, err := json.Marshal(baseRequestbody)
	if err != nil {
		return nil, err
	}

	ylog.Debug("< request", "body", string(jsonBody))

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// authentication
	// req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
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

	var respBodyStruct RespBody
	err = json.Unmarshal(respBody, &respBodyStruct)
	if err != nil {
		return nil, err
	}

	choice := respBodyStruct.Choices[0]

	ylog.Debug(">>finish_reason", "reason", choice.FinishReason)
	if choice.FinishReason == "tool_calls" {
		ylog.Warn("TODO: should re-request with this response")
	}

	calls := respBodyStruct.Choices[0].Message.ToolCalls
	content := respBodyStruct.Choices[0].Message.Content

	ylog.Debug("--response calls", "calls", calls)

	result := &ai.InvokeResponse{}

	// finish reason
	result.FinishReason = choice.FinishReason

	// record usage
	result.TokenUsage = ai.TokenUsage{
		PromptTokens:     respBodyStruct.Usage.PromptTokens,
		CompletionTokens: respBodyStruct.Usage.CompletionTokens,
	}

	if len(calls) == 0 {
		result.Content = content
		return result, ai.ErrNoFunctionCall
	}

	// functions may be more than one
	// slog.Info("tool calls", "calls", calls, "mapTools", mapTools)
	for _, call := range calls {
		for tag, tc := range tcs {
			if tc.Equal(&call) {
				// Use toolCalls because tool_id is required in the following llm request
				if result.ToolCalls == nil {
					result.ToolCalls = make(map[uint32][]*ai.ToolCall)
				}
				// Create a new variable to hold the current call
				currentCall := call
				result.ToolCalls[tag] = append(result.ToolCalls[tag], &currentCall)
			}
		}
	}

	// sfn maybe disconnected, so we need to check if there is any function call
	if len(result.ToolCalls) == 0 {
		return nil, ai.ErrNoFunctionCall
	}
	return result, nil
}
