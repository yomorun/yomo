// Package anthropic is the anthropic llm provider, see https://docs.anthropic.com
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
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
	// send anthropic request
	result, err := p.client.Messages.New(ctx, p.convertMessageNewParams(req))
	if err != nil {
		ylog.Error("anthropic api request", "err", err)
		return openai.ChatCompletionResponse{}, err
	}
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
	choice.FinishReason = convertToOpenAIFinishReason(result.StopReason)
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
	// send anthropic stream request
	stream := p.client.Messages.NewStreaming(ctx, p.convertMessageNewParams(req))
	// stream options
	includeUsage := false
	if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
		includeUsage = true
	}
	// response recver
	recv := &recver{
		underlying:   stream,
		includeUsage: includeUsage,
	}

	return recv, nil
}

// recver is the response receiver for anthropic stream
type recver struct {
	id           string
	model        string
	includeUsage bool
	inputTokens  int
	outputTokens int
	underlying   *ssestream.Stream[anthropic.MessageStreamEvent]
	toolCalls    []openai.ToolCall
}

// Recv implements provider.ResponseRecver.
func (r *recver) Recv() (response openai.ChatCompletionStreamResponse, err error) {
	resp := openai.ChatCompletionStreamResponse{
		Object:  "chat.completion.chunk",
		Choices: make([]openai.ChatCompletionStreamChoice, 0),
	}
	// event
	hasMore := r.underlying.Next()
	if !hasMore {
		// response end
		return resp, io.EOF
	}
	// current event processing
	event := r.underlying.Current()
	choiceIndex := len(resp.Choices)
	toolCallIndex := len(r.toolCalls)

	switch event.Type {
	case anthropic.MessageStreamEventTypeMessageStart:
		r.id = event.Message.ID
		r.model = event.Message.Model
		r.inputTokens = int(event.Message.Usage.InputTokens)
		ylog.Debug("anthropic message start", "event", event.Type, "id", r.id, "model", r.model, "input_tokens", r.inputTokens)
	case anthropic.MessageStreamEventTypeMessageDelta:
		r.outputTokens = int(event.Usage.OutputTokens)
		ylog.Debug("anthropic message delta", "event", event.Type, "output_tokens", r.outputTokens)
	case anthropic.MessageStreamEventTypeMessageStop:
		resp.ID = r.id
		resp.Model = r.model
		resp.Created = time.Now().Unix()
		// usage
		if r.includeUsage {
			resp.Usage = &openai.Usage{
				PromptTokens:     r.inputTokens,
				CompletionTokens: r.outputTokens,
				TotalTokens:      r.inputTokens + r.outputTokens,
			}
			ylog.Debug("anthropic message stop", "usage", resp.Usage)
		}
		ylog.Debug("anthropic message stop", "event", event.Type, "response", fmt.Sprintf("%+v", resp))
		return resp, nil
	case anthropic.MessageStreamEventTypeContentBlockStart:
		ylog.Debug("anthropic content block start", "event", event.Type)
		switch block := event.ContentBlock.(type) {
		case anthropic.ContentBlockStartEventContentBlock:
			ylog.Debug("anthropic content block type", "block_type", block.Type)
			// tool use
			// if toolUseBlock,ok := block.AsUnion().(anthropic.ToolUseBlock); ok {
			if block.Type == anthropic.ContentBlockStartEventContentBlockTypeToolUse {
				toolCall := openai.ToolCall{
					Index: &toolCallIndex,
					ID:    block.ID,
					Type:  openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name: block.Name,
					},
				}
				// new tool call
				ylog.Debug("anthropic tool use", "event", event.Type, fmt.Sprintf("tool_call[%d]", toolCallIndex), fmt.Sprintf("%+v", toolCall))
				r.toolCalls = append(r.toolCalls, toolCall)
				choice := openai.ChatCompletionStreamChoice{
					Index: choiceIndex,
					Delta: openai.ChatCompletionStreamChoiceDelta{ToolCalls: r.toolCalls},
				}
				resp.Choices = append(resp.Choices, choice)
			}
		}
	}
	// response
	resp.ID = r.id
	resp.Model = r.model
	resp.Created = time.Now().Unix()
	// delta processing
	switch delta := event.Delta.(type) {
	case anthropic.ContentBlockDeltaEventDelta:
		choice := openai.ChatCompletionStreamChoice{Index: choiceIndex}
		// delta type
		deltaType := delta.Type
		switch deltaType {
		// text
		case anthropic.ContentBlockDeltaEventDeltaTypeTextDelta:
			choice.Delta = openai.ChatCompletionStreamChoiceDelta{
				Content: delta.Text,
				Role:    openai.ChatMessageRoleAssistant,
			}
		// tool use
		case anthropic.ContentBlockDeltaEventDeltaTypeInputJSONDelta:
			// tool call already added in ContentBlockStartEvent
			if toolCallIndex > 0 {
				index := toolCallIndex - 1
				toolCall := openai.ToolCall{
					Index: &index,
					Type:  openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Arguments: string(delta.PartialJSON),
					},
				}
				// partial tool call
				ylog.Debug("anthropic input json delta",
					"event", event.Type,
					"partial_json", delta.PartialJSON,
					fmt.Sprintf("tool_call[%d]", toolCallIndex), fmt.Sprintf("%+v", toolCall),
				)
				toolCalls := []openai.ToolCall{toolCall}
				choice.Delta = openai.ChatCompletionStreamChoiceDelta{ToolCalls: toolCalls}
			}
		}
		// add choice
		resp.Choices = append(resp.Choices, choice)
	// delta stop
	case anthropic.MessageDeltaEventDelta:
		choice := openai.ChatCompletionStreamChoice{
			Index:        choiceIndex,
			FinishReason: convertToOpenAIFinishReason(anthropic.MessageStopReason(delta.StopReason)),
		}
		ylog.Debug("anthropic content block delta", "event", event.Type, "finish_reason", choice.FinishReason)
		resp.Choices = append(resp.Choices, choice)
	}
	// stream error
	if err := r.underlying.Err(); err != nil {
		ylog.Error("anthropic stream error", "err", err)
		return resp, r.underlying.Err()
	}
	// chunkResp, _ := json.Marshal(resp)
	// ylog.Warn("openai response", "chunk_response", string(chunkResp))
	return resp, nil
}

// convertMessageNewParams converts openai.ChatCompletionRequest to anthropic.MessageNewParams
func (p *Provider) convertMessageNewParams(req openai.ChatCompletionRequest) anthropic.MessageNewParams {
	// model
	if req.Model == "" {
		req.Model = p.Model
	}
	// default max tokens
	if req.MaxTokens == 0 {
		req.MaxTokens = DefaultMaxTokens
	}

	msgs := make([]anthropic.MessageParam, 0)
	systemMsgs := make([]anthropic.TextBlockParam, 0)
	tools := make([]anthropic.ToolParam, 0)
	toolResult := []anthropic.MessageParamContentUnion{}

	// tools
	for _, tool := range req.Tools {
		// only support function type
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
			if len(msg.ToolCalls) > 0 {
				ylog.Debug("openai request", "tool_calls", len(msg.ToolCalls))
				toolUses := make([]anthropic.MessageParamContentUnion, 0)
				for _, toolCall := range msg.ToolCalls {
					var args map[string]any
					if len(toolCall.Function.Arguments) > 0 {
						err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
						if err != nil {
							ylog.Error("anthropic tool use unmarshal input", "err", err)
							continue
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

	return anthropic.MessageNewParams{
		Model:         anthropic.F(req.Model),
		MaxTokens:     anthropic.F(int64(req.MaxTokens)),
		Messages:      anthropic.F(msgs),
		System:        anthropic.F(systemMsgs),
		Tools:         anthropic.F(tools),
		TopP:          anthropic.F(float64(req.TopP)),
		Temperature:   anthropic.F(float64(req.Temperature)),
		StopSequences: anthropic.F(req.Stop),
		// ToolChoice unsupported
		// TopK unsupported
	}
}

// convertToOpenAIFinishReason convert anthropic.MessageStopReason to openai.FinishReason
func convertToOpenAIFinishReason(reason anthropic.MessageStopReason) openai.FinishReason {
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
