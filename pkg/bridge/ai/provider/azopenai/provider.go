package azopenai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	_ "github.com/joho/godotenv/autoload"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
)

var (
	// tools             map[uint32]ai.ToolCall
	fns sync.Map
	// mu                sync.Mutex
	ErrNoFunctionCall = errors.New("no function call")
)

// RequestMessage is the message in Request
type ReqMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// RequestBody is the request body
type ReqBody struct {
	Messages []ReqMessage  `json:"messages"`
	Tools    []ai.ToolCall `json:"tools"` // chatCompletionTool
	// ToolChoice string    `json:"tool_choice"` // chatCompletionFunction
}

// Resp is the response body
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
	APIKey      string
	APIEndpoint string
}

type connectedFn struct {
	connID string
	tag    uint32
	tc     ai.ToolCall
}

func init() {
	// tools = make(map[uint32]ai.ToolCall)
	fns = sync.Map{}
	// ai.RegisterProvider(NewAzureOpenAIProvider("api-key", "api-endpoint"))
	// TEST: for test
	// bridgeai.RegisterProvider(New())
}

// NewAzureOpenAIProvider creates a new AzureOpenAIProvider
func NewAzureOpenAIProvider(apiKey string, apiEndpoint string) *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		APIKey:      apiKey,
		APIEndpoint: apiEndpoint,
	}
}

// New creates a new AzureOpenAIProvider
func New() *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		APIKey:      os.Getenv("AZURE_OPENAI_API_KEY"),
		APIEndpoint: os.Getenv("AZURE_OPENAI_API_ENDPOINT"),
	}
}

// Name returns the name of the provider
func (p *AzureOpenAIProvider) Name() string {
	return "azopenai"
}

// GetChatCompletions get chat completions for ai service
func (p *AzureOpenAIProvider) GetChatCompletions(userInstruction string) (*ai.ChatCompletionsResponse, error) {
	isEmpty := true
	fns.Range(func(key, value interface{}) bool {
		isEmpty = false
		return false
	})

	if isEmpty {
		ylog.Error("-----tools is empty")
		return &ai.ChatCompletionsResponse{Content: "no toolcalls"}, ErrNoFunctionCall
	}

	// messages
	messages := []ReqMessage{
		{Role: "system", Content: `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous. If you don't know the answer, stop the conversation by saying "no func call".`},
		{Role: "user", Content: userInstruction},
	}

	// prepare tools
	toolCalls := make([]ai.ToolCall, 0)
	// for _, v := range tools {
	// 	toolCalls = append(toolCalls, v)
	// }
	fns.Range(func(key, value interface{}) bool {
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

	req, err := http.NewRequest("POST", p.APIEndpoint, bytes.NewBuffer(jsonBody))
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

	ylog.Debug("--response calls", "calls", len(calls), "content", content)

	result := &ai.ChatCompletionsResponse{}
	if len(calls) == 0 {
		result.Content = content
		return result, ErrNoFunctionCall
	}

	// functions may be more than one
	// slog.Info("tool calls", "calls", calls, "mapTools", mapTools)
	for _, call := range calls {
		fns.Range(func(key, value interface{}) bool {
			fn := value.(*connectedFn)
			if fn.tc.Equal(&call) {
				if result.Functions == nil {
					result.Functions = make(map[uint32][]*ai.FunctionDefinition)
				}
				// TODO: should push the `call` instead of `call.Function` as describes in
				// https://cookbook.openai.com/examples/function_calling_with_an_openapi_spec
				result.Functions[fn.tag] = append(result.Functions[fn.tag], call.Function)
			}
			return true
		})
		// for tag, tool := range tools {
		// 	ylog.Debug("---compare", "the-calls-type", call.Type, "the-calls-name", call.Function.Name, "the-tool-type", tool.Type, "the-tool-name", tool.Function.Name)
		// 	if tool.Equal(&call) {
		// 		if result.Functions == nil {
		// 			result.Functions = make(map[uint32][]*ai.FunctionDefinition)
		// 		}
		// 		result.Functions[tag] = append(result.Functions[tag], call.Function)
		// 	}
		// }
	}

	ylog.Debug("---result", "result_functions_count", len(result.Functions))

	// sfn maybe disconnected, so we need to check if there is any function call
	if len(result.Functions) == 0 {
		return nil, ErrNoFunctionCall
	}
	return result, nil
}

// RegisterFunction register function
func (p *AzureOpenAIProvider) RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID string) error {
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
func (p *AzureOpenAIProvider) UnregisterFunction(name string, connID string) error {
	fns.Delete(connID)
	return nil
}

// ListToolCalls list tool functions
func (p *AzureOpenAIProvider) ListToolCalls() (map[uint32]ai.ToolCall, error) {
	tmp := make(map[uint32]ai.ToolCall)
	fns.Range(func(key, value any) bool {
		fn := value.(*connectedFn)
		tmp[fn.tag] = fn.tc
		return true
	})

	return tmp, nil
}

// GetOverview get overview for ai service
func (p *AzureOpenAIProvider) GetOverview() (*ai.OverviewResponse, error) {
	isEmpty := true
	fns.Range(func(key, value any) bool {
		isEmpty = false
		return false
	})

	result := &ai.OverviewResponse{
		Functions: make(map[uint32]*ai.FunctionDefinition),
	}

	if isEmpty {
		return result, nil
	}

	fns.Range(func(key, value any) bool {
		fn := value.(*connectedFn)
		result.Functions[fn.tag] = fn.tc.Function
		return true
	})

	return result, nil
}
