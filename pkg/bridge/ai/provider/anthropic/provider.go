// Package anthropic is the anthropic llm provider, see https://docs.anthropic.com
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

const (
	// DefaultMaxTokens is the default max tokens for completion
	DefaultMaxTokens = 2048
)

// check if implements ai.Provider
var _ provider.LLMProvider = &Provider{}

// Provider is the provider for anthropic models
type Provider struct {
	APIKey string
	Model  string
	client *anthropic.Client
}

// NewProvider creates a new anthropic provider
func NewProvider(apiKey string, model string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if model == "" {
		model = os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = anthropic.ModelClaude_3_5_Sonnet_20240620
		}
	}

	return &Provider{
		APIKey: apiKey,
		Model:  model,
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "anthropic"
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(
	ctx context.Context,
	req openai.ChatCompletionRequest,
	_ metadata.M,
) (openai.ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.Model
	}
	// default max tokens
	if req.MaxTokens == 0 {
		req.MaxTokens = DefaultMaxTokens
	}
	// convert open ai request messages to anthropic messages
	msgs := make([]anthropic.MessageParam, 0)
	systemMsgs := make([]anthropic.TextBlockParam, 0)
	tools := make([]anthropic.ToolParam, 0)
	toolResult := []anthropic.MessageParamContentUnion{}

	// tools
	for _, tool := range req.Tools {
		if tool.Type == openai.ToolTypeFunction {
			tools = append(tools, anthropic.ToolParam{
				Name:        anthropic.F(tool.Function.Name),
				Description: anthropic.F(tool.Function.Description),
				InputSchema: anthropic.F(tool.Function.Parameters),
			})
		}
	}
	ylog.Debug("anthropic tools", "tools", fmt.Sprintf("%+v", tools))

	// messages
	for _, msg := range req.Messages {
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case openai.ChatMessageRoleAssistant:
			// tool use, check if there are tool calls
			ylog.Debug("openai request", "tool_calls", len(msg.ToolCalls))
			if len(msg.ToolCalls) > 0 {
				toolUses := make([]anthropic.MessageParamContentUnion, 0)
				for _, toolCall := range msg.ToolCalls {
					var args map[string]any
					if len(toolCall.Function.Arguments) > 0 {
						err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
						if err != nil {
							// TODO: handle error
							ylog.Error("anthropic tool use unmarshal input", "err", err)
						}
					}
					toolUse := anthropic.NewToolUseBlockParam(toolCall.ID, toolCall.Function.Name, args)
					ylog.Debug("anthropic tool use", "tool_use", fmt.Sprintf("%+v", toolUse))
					toolUses = append(toolUses, toolUse)
				}
				msgs = append(msgs, anthropic.NewAssistantMessage(toolUses...))
			} else { // normal assistant message
				msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
			}
		case openai.ChatMessageRoleSystem:
			systemMsgs = append(systemMsgs, anthropic.NewTextBlock(msg.Content))
		// tool result
		case openai.ChatMessageRoleTool:
			toolResult = append(toolResult, anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false))
		}
	}
	// add tool result to user messages
	if len(toolResult) > 0 {
		msgs = append(msgs, anthropic.NewUserMessage(toolResult...))
	}

	// send anthropic request
	result, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:         anthropic.F(req.Model),
		MaxTokens:     anthropic.F(int64(req.MaxTokens)),
		Messages:      anthropic.F(msgs),
		System:        anthropic.F(systemMsgs),
		Tools:         anthropic.F(tools),
		TopP:          anthropic.F(float64(req.TopP)),
		Temperature:   anthropic.F(float64(req.Temperature)),
		StopSequences: anthropic.F(req.Stop),
		// ToolChoice:
		// TopK:
	})
	if err != nil {
		ylog.Error("anthropic.Messages.New", "err", err)
		return openai.ChatCompletionResponse{}, err
	}
	// ylog.Debug("anthropic raw response", "result", fmt.Sprintf("%+v", result))

	// convert anthropic.MessageResponse to openai.ChatCompletionResponse
	resp := openai.ChatCompletionResponse{
		ID:                result.ID,
		Model:             result.Model,
		Object:            "chat.completion",
		Created:           time.Now().Unix(),
		Choices:           make([]openai.ChatCompletionChoice, 0),
		SystemFingerprint: "yomo_anthropic",
	}
	message := openai.ChatCompletionMessage{
		Role:      openai.ChatMessageRoleAssistant,
		ToolCalls: make([]openai.ToolCall, 0),
	}
	toolCallIndex := 0
	for _, content := range result.Content {
		switch content.Type {
		// switch content.AsUnion().(type) {
		// text
		case anthropic.ContentBlockTypeText:
			// case anthropic.TextBlock:
			message.Content = content.Text
			// tool use
		case anthropic.ContentBlockTypeToolUse:
			// case anthropic.ToolUseBlock:
			i := toolCallIndex
			ylog.Debug("anthropic tool use ", "function", content.Name, "arguments", string(content.Input))
			message.ToolCalls = append(message.ToolCalls, openai.ToolCall{
				Index: &i,
				ID:    content.ID,
				Type:  openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      content.Name,
					Arguments: string(content.Input),
				},
			})
			toolCallIndex++
		}
	}
	// choice
	choice := openai.ChatCompletionChoice{Message: message}
	// finish reasson
	choice.FinishReason = convertFinishReason(result.StopReason)
	resp.Choices = append(resp.Choices, choice)
	// usage
	// BUG:
	// total tokens = input tokens + output tokens
	// #1 429, 139, 568
	// #2 613, 171, 784
	// = 1042, 310, 1352
	// actual:
	// "prompt_tokens": 1042,
	// "completion_tokens": 310,
	// "total_tokens": 923
	resp.Usage = openai.Usage{
		PromptTokens:     int(result.Usage.InputTokens),
		CompletionTokens: int(result.Usage.OutputTokens),
		TotalTokens:      int(result.Usage.InputTokens + result.Usage.OutputTokens),
	}

	return resp, nil
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(
	ctx context.Context,
	req openai.ChatCompletionRequest,
	_ metadata.M) (provider.ResponseRecver, error) {
	if req.Model == "" {
		req.Model = p.Model
	}
	// TODO: anthropic stream request
	return nil, nil
}

func convertFinishReason(reason anthropic.MessageStopReason) openai.FinishReason {
	if reason.IsKnown() {
		switch reason {
		case anthropic.MessageStopReasonEndTurn:
			return openai.FinishReasonStop
		case anthropic.MessageStopReasonMaxTokens:
			return openai.FinishReasonLength
		case anthropic.MessageStopReasonStopSequence:
			return openai.FinishReasonStop
		case anthropic.MessageStopReasonToolUse:
			return openai.FinishReasonToolCalls
		}
	}
	return openai.FinishReason(string(reason))
}
