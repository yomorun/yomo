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

	"golang.org/x/exp/slog"

	_ "github.com/joho/godotenv/autoload"
	"github.com/yomorun/yomo/ai"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

var (
	tools             map[string]map[uint32][]ai.ToolCall
	mu                sync.Mutex
	ErrNoFunctionCall = errors.New("no function call")
)

// RequestMessage is the message in Request
type ReqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

type AzureOpenAIProvider struct {
	APIKey      string
	APIEndpoint string
}

func NewAzureOpenAIProvider(apiKey string, apiEndpoint string) *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		APIKey:      apiKey,
		APIEndpoint: apiEndpoint,
	}
}

func New() *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		APIKey:      os.Getenv("AZURE_OPENAI_API_KEY"),
		APIEndpoint: os.Getenv("AZURE_OPENAI_API_ENDPOINT"),
	}
}

func (p *AzureOpenAIProvider) Name() string {
	return "azopenai"
}

func (p *AzureOpenAIProvider) GetChatCompletions(appID string, tag uint32, userPrompt string) (*ai.ChatCompletionsResponse, error) {
	// messages
	messages := []ReqMessage{
		{Role: "system", Content: `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. If you don't know the answer, stop the conversation by saying "no func call".`},
		{Role: "user", Content: userPrompt},
	}
	// tools
	tools, err := p.ListToolCalls(appID, tag)
	if err != nil {
		return nil, err
	}
	body := ReqBody{Messages: messages, Tools: tools}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	// slog.Info("request url", "url", p.APIEndpoint)
	// slog.Info("request api key", "api-key", p.APIKey)
	// slog.Info("request body", "body", string(jsonBody))

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
	// slog.Info("response body", "body", string(respBody))
	if resp.StatusCode >= 400 {
		// log.Println(resp.StatusCode, string(respBody))
		// {"error":{"code":"429","message": "Requests to the ChatCompletions_Create Operation under Azure OpenAI API version 2023-12-01-preview have exceeded token rate limit of your current OpenAI S0 pricing tier. Please retry after 22 seconds. Please go here: https://aka.ms/oai/quotaincrease if you would like to further increase the default rate limit."}}
		return nil, errors.New(fmt.Sprintf("ai response status code is %d", resp.StatusCode))
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

	calls := respBodyStruct.Choices[0].Message.ToolCalls
	content := respBodyStruct.Choices[0].Message.Content
	result := &ai.ChatCompletionsResponse{}
	if len(calls) == 0 {
		result.Content = content
		return result, ErrNoFunctionCall
	}
	// functions may be more than one
	for _, call := range calls {
		result.Functions = append(result.Functions, call.Function)
	}
	return result, nil
}

// RegisterFunction register function
func (p *AzureOpenAIProvider) RegisterFunction(appID string, tag uint32, functionDefinition []byte) error {
	mu.Lock()
	defer mu.Unlock()
	appTools := tools[appID]
	if appTools == nil {
		appTools = make(map[uint32][]ai.ToolCall)
	}
	fd := ai.FunctionDefinition{}
	err := json.Unmarshal(functionDefinition, &fd)
	if err != nil {
		slog.Error("unmarshal function definition", "error", err)
		return err
	}
	appTools[tag] = append(
		appTools[tag],
		ai.ToolCall{
			Type:     "function",
			Function: &fd,
		},
	)
	tools[appID] = appTools
	return nil
}

// UnregisterFunction unregister function
func (p *AzureOpenAIProvider) UnregisterFunction(appID string, tag uint32) error {
	mu.Lock()
	defer mu.Unlock()
	appTools := tools[appID]
	if appTools != nil {
		delete(appTools, tag)
	}
	return nil
}

// ListToolCalls list tool calls
func (p *AzureOpenAIProvider) ListToolCalls(appID string, tag uint32) ([]ai.ToolCall, error) {
	appTools, ok := tools[appID]
	if !ok {
		return nil, nil
	}
	return appTools[tag], nil
}

func init() {
	tools = make(map[string]map[uint32][]ai.ToolCall)
	// ai.RegisterProvider(NewAzureOpenAIProvider("api-key", "api-endpoint"))
	// TEST: for test
	bridgeai.RegisterProvider(New())
}
