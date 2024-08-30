// Package anthropic is the anthropic llm provider, see https://docs.anthropic.com
package anthropic

import (
	"context"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"

	provider "github.com/yomorun/yomo/pkg/bridge/ai/provider"
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
	// TODO: convert openai.ChatCompletionRequest to anthropic.MessageRequest
	// convert open ai request messages to anthropic messages
	msgs := make([]anthropic.MessageParam, 0)
	systemMsgs := make([]anthropic.TextBlockParam, 0)
	tools := make([]anthropic.ToolParam, 0)
	toolResult := make([]anthropic.ToolResultBlockParam, 0)

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

	// messages
	for _, msg := range req.Messages {
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case openai.ChatMessageRoleAssistant:
			// tool use, check if there are tool calls
			if len(msg.ToolCalls) > 0 {
				for _, toolCall := range msg.ToolCalls {
					msgs = append(
						msgs,
						anthropic.NewAssistantMessage(anthropic.NewToolUseBlockParam(toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)),
					)
				}
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
	for _, tr := range toolResult {
		msgs = append(msgs, anthropic.NewUserMessage(tr))
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
		return openai.ChatCompletionResponse{}, err
	}

	// convert anthropic.MessageResponse to openai.ChatCompletionResponse
	resp := openai.ChatCompletionResponse{
		ID:                result.ID,
		Model:             result.Model,
		Object:            "chat.completion",
		Created:           time.Now().Unix(),
		Choices:           make([]openai.ChatCompletionChoice, 0),
		SystemFingerprint: "anthropic",
	}
	message := openai.ChatCompletionMessage{
		Role:      openai.ChatMessageRoleAssistant,
		ToolCalls: make([]openai.ToolCall, 0),
	}
	toolCallIndex := 0
	for _, content := range result.Content {
		switch content.Type {
		// text
		case anthropic.ContentBlockTypeText:
			message.Content = content.Text
			// tool use
		case anthropic.ContentBlockTypeToolUse:
			i := toolCallIndex
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
