// Package azopenai is used to provide the Azure OpenAI service
package azopenai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

// ReqMessage is the message in Request
type ReqMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ReqBody is the request body
type ReqBody struct {
	Messages []ReqMessage  `json:"messages"`
	Tools    []ai.ToolCall `json:"tools"` // chatCompletionTool
	// ToolChoice string    `json:"tool_choice"` // chatCompletionFunction
}

// RespBody is the response body
type RespBody struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"`
	Created           int          `json:"created"`
	Model             string       `json:"model"`
	Choices           []RespChoice `json:"choices"`
	Usage             RespUsage    `json:"usage"`
	SystemFingerprint string       `json:"system_fingerprint"`
}

// RespMessage is the message in Response
type RespMessage struct {
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	ToolCalls []ai.ToolCall `json:"tool_calls"`
}

// RespChoice is used to indicate the choice in Response by `FinishReason`
type RespChoice struct {
	FinishReason string      `json:"finish_reason"`
	Index        int         `json:"index"`
	Message      RespMessage `json:"message"`
}

// RespUsage is the token usage in Response
type RespUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// AzureOpenAIProvider is the provider for Azure OpenAI
type AzureOpenAIProvider struct {
	APIKey       string
	APIEndpoint  string
	DeploymentID string
	APIVersion   string
}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(apiKey string, apiEndpoint string, deploymentID string, apiVersion string) *AzureOpenAIProvider {
	if apiKey == "" {
		apiKey = os.Getenv("AZURE_OPENAI_API_KEY")
	}
	if apiEndpoint == "" {
		apiEndpoint = os.Getenv("AZURE_OPENAI_API_ENDPOINT")
	}
	if deploymentID == "" {
		deploymentID = os.Getenv("AZURE_OPENAI_DEPLOYMENT_ID")
	}
	if apiVersion == "" {
		apiVersion = os.Getenv("AZURE_OPENAI_API_VERSION")
	}
	return &AzureOpenAIProvider{
		APIKey:       apiKey,
		APIEndpoint:  apiEndpoint,
		DeploymentID: deploymentID,
		APIVersion:   apiVersion,
	}
}

// Name returns the name of the provider
func (p *AzureOpenAIProvider) Name() string {
	return "azopenai"
}

// GetChatCompletions get chat completions for ai service
func (p *AzureOpenAIProvider) GetChatCompletions(userInstruction string, md metadata.M) (*ai.InvokeResponse, error) {
	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return nil, err
	}

	if len(tcs) == 0 {
		ylog.Error("tools is empty")
		return &ai.InvokeResponse{Content: "no toolcalls"}, ai.ErrNoFunctionCall
	}

	// messages
	messages := []ReqMessage{
		{Role: "system", Content: `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous. If you don't know the answer, stop the conversation by saying "no func call".`},
		{Role: "user", Content: userInstruction},
	}

	// prepare tools
	toolCalls := make([]ai.ToolCall, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = tc
		idx++
	}

	body := ReqBody{Messages: messages, Tools: toolCalls}

	ylog.Debug("request", "tools", len(toolCalls), "messages", messages)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", p.APIEndpoint, p.DeploymentID, p.APIVersion)
	ylog.Debug("chat completions request", "url", url)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", p.APIKey)

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
	ylog.Debug("response", "body", respBody)

	// slog.Info("response body", "body", string(respBody))
	if resp.StatusCode >= 400 {
		// log.Println(resp.StatusCode, string(respBody))
		// {"error":{"code":"429","message": "Requests to the ChatCompletions_Create Operation under Azure OpenAI API version 2023-12-01-preview have exceeded token rate limit of your current OpenAI S0 pricing tier. Please retry after 22 seconds. Please go here: https://aka.ms/oai/quotaincrease if you would like to further increase the default rate limit."}}
		return nil, fmt.Errorf("ai response status code is %d", resp.StatusCode)
	}

	var respBodyStruct RespBody
	err = json.Unmarshal(respBody, &respBodyStruct)
	if err != nil {
		return nil, err
	}
	// fmt.Println(string(respBody))
	// TODO: record usage
	// usage := respBodyStruct.Usage
	// log.Printf("Token Usage: %+v\n", usage)

	choice := respBodyStruct.Choices[0]
	ylog.Debug(">>finish_reason", "reason", choice.FinishReason)

	calls := respBodyStruct.Choices[0].Message.ToolCalls
	content := respBodyStruct.Choices[0].Message.Content

	ylog.Debug("--response calls", "calls", calls)

	result := &ai.InvokeResponse{}
	if len(calls) == 0 {
		result.Content = content
		return result, ai.ErrNoFunctionCall
	}

	// functions may be more than one
	for _, call := range calls {
		for tag, tc := range tcs {
			if tc.Equal(&call) {
				// Use toolCalls because tool_id is required in the following llm request
				if result.ToolCalls == nil {
					result.ToolCalls = make(map[uint32][]*ai.ToolCall)
				}

				// push the `call` instead of `call.Function` as describes in
				// https://cookbook.openai.com/examples/function_calling_with_an_openapi_spec
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
