// Package azopenai is used to provide the Azure OpenAI service
package azopenai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
)

var fns sync.Map

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

type connectedFn struct {
	connID uint64
	tag    uint32
	tc     ai.ToolCall
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
func (p *AzureOpenAIProvider) GetChatCompletions(userInstruction string) (*ai.InvokeResponse, error) {
	isEmpty := true
	fns.Range(func(_, _ interface{}) bool {
		isEmpty = false
		return false
	})

	if isEmpty {
		ylog.Error("-----tools is empty")
		return &ai.InvokeResponse{Content: "no toolcalls"}, ai.ErrNoFunctionCall
	}

	// messages
	messages := []ReqMessage{
		{Role: "system", Content: `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous. If you don't know the answer, stop the conversation by saying "no func call".`},
		{Role: "user", Content: userInstruction},
	}

	// prepare tools
	toolCalls := make([]ai.ToolCall, 0)
	fns.Range(func(_, value interface{}) bool {
		fn := value.(*connectedFn)
		toolCalls = append(toolCalls, fn.tc)
		return true
	})

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
	// slog.Info("tool calls", "calls", calls, "mapTools", mapTools)
	for _, call := range calls {
		fns.Range(func(_, value interface{}) bool {
			fn := value.(*connectedFn)
			if fn.tc.Equal(&call) {
				// Use toolCalls because tool_id is required in the following llm request
				if result.ToolCalls == nil {
					result.ToolCalls = make(map[uint32][]*ai.ToolCall)
				}

				// push the `call` instead of `call.Function` as describes in
				// https://cookbook.openai.com/examples/function_calling_with_an_openapi_spec
				currentCall := call
				result.ToolCalls[fn.tag] = append(result.ToolCalls[fn.tag], &currentCall)
			}
			return true
		})
	}

	// sfn maybe disconnected, so we need to check if there is any function call
	if len(result.ToolCalls) == 0 {
		return nil, ai.ErrNoFunctionCall
	}
	return result, nil
}

// RegisterFunction register function
func (p *AzureOpenAIProvider) RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64) error {
	fns.Store(connID, &connectedFn{
		connID: connID,
		tag:    tag,
		tc: ai.ToolCall{
			Type:     "function",
			Function: functionDefinition,
		},
	})

	return nil
}

// UnregisterFunction unregister function
// Be careful: a function can have multiple instances, remove the offline instance only.
func (p *AzureOpenAIProvider) UnregisterFunction(_ string, connID uint64) error {
	fns.Delete(connID)
	return nil
}

// ListToolCalls list tool functions
func (p *AzureOpenAIProvider) ListToolCalls() (map[uint32]ai.ToolCall, error) {
	tmp := make(map[uint32]ai.ToolCall)
	fns.Range(func(_, value any) bool {
		fn := value.(*connectedFn)
		tmp[fn.tag] = fn.tc
		return true
	})

	return tmp, nil
}

// GetOverview get overview for ai service
func (p *AzureOpenAIProvider) GetOverview() (*ai.OverviewResponse, error) {
	isEmpty := true
	fns.Range(func(_, _ any) bool {
		isEmpty = false
		return false
	})

	result := &ai.OverviewResponse{
		Functions: make(map[uint32]*ai.FunctionDefinition),
	}

	if isEmpty {
		return result, nil
	}

	fns.Range(func(_, value any) bool {
		fn := value.(*connectedFn)
		result.Functions[fn.tag] = fn.tc.Function
		return true
	})

	return result, nil
}
