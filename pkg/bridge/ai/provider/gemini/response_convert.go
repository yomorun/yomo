package gemini

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/generative-ai-go/genai"
	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/pkg/id"
)

func convertToResponse(in *genai.GenerateContentResponse, model string) (out openai.ChatCompletionResponse) {
	out = openai.ChatCompletionResponse{
		ID:      "chatcmpl-" + id.New(29),
		Model:   model,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: make([]openai.ChatCompletionChoice, 0),
	}

	if in.UsageMetadata != nil {
		out.Usage = openai.Usage{
			PromptTokens:     int(in.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(in.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(in.UsageMetadata.TotalTokenCount),
		}
	}

	count := 0
	toolCalls := make([]openai.ToolCall, 0)
	for _, candidate := range in.Candidates {
		for _, part := range candidate.Content.Parts {
			index := count
			switch pp := part.(type) {
			case genai.Text:
				out.Choices = append(out.Choices, openai.ChatCompletionChoice{
					Index: int(index),
					Message: openai.ChatCompletionMessage{
						Content: string(pp),
						Role:    openai.ChatMessageRoleUser,
					},
					FinishReason: toOpenAIFinishReason(candidate.FinishReason),
				})
			case genai.FunctionCall:
				args, _ := json.Marshal(pp.Args)
				toolCalls = append(toolCalls, openai.ToolCall{
					Index:    genai.Ptr(int(index)),
					ID:       fmt.Sprintf("%s-%d", pp.Name, index),
					Type:     openai.ToolTypeFunction,
					Function: openai.FunctionCall{Name: pp.Name, Arguments: string(args)},
				})
			}
			count++
		}
	}

	if len(toolCalls) > 0 {
		out.Choices = append(out.Choices, openai.ChatCompletionChoice{
			Message: openai.ChatCompletionMessage{
				ToolCalls: toolCalls,
				Role:      openai.ChatMessageRoleAssistant,
			},
			FinishReason: openai.FinishReasonToolCalls,
		})
	}

	return
}

func convertToStreamResponse(id string, in *genai.GenerateContentResponse, model string, usage *openai.Usage) openai.ChatCompletionStreamResponse {
	out := openai.ChatCompletionStreamResponse{
		ID:      id,
		Model:   model,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Choices: make([]openai.ChatCompletionStreamChoice, 0),
	}

	if in.UsageMetadata != nil {
		usage.PromptTokens = int(in.UsageMetadata.PromptTokenCount)
		usage.CompletionTokens += int(in.UsageMetadata.CandidatesTokenCount)
	}

	count := 0
	toolCalls := make([]openai.ToolCall, 0)

	for _, candidate := range in.Candidates {
		parts := candidate.Content.Parts
		for _, part := range parts {
			index := count
			switch pp := part.(type) {
			case genai.Text:
				out.Choices = append(out.Choices, openai.ChatCompletionStreamChoice{
					Index: index,
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Content: string(pp),
						Role:    openai.ChatMessageRoleUser,
					},
					FinishReason: toOpenAIFinishReason(candidate.FinishReason),
				})
			case genai.FunctionCall:
				args, _ := json.Marshal(pp.Args)

				toolCalls = append(toolCalls, openai.ToolCall{
					Index:    genai.Ptr(int(index)),
					ID:       fmt.Sprintf("%s-%d", pp.Name, index),
					Type:     openai.ToolTypeFunction,
					Function: openai.FunctionCall{Name: pp.Name, Arguments: string(args)},
				})
			}
			count++
		}
	}
	if len(toolCalls) > 0 {
		out.Choices = append(out.Choices, openai.ChatCompletionStreamChoice{
			Delta: openai.ChatCompletionStreamChoiceDelta{
				ToolCalls: toolCalls,
				Role:      openai.ChatMessageRoleAssistant,
			},
			FinishReason: openai.FinishReasonToolCalls,
		})
	}

	return out
}

var mapFinishReason = map[genai.FinishReason]openai.FinishReason{
	genai.FinishReasonUnspecified: openai.FinishReasonNull,
	genai.FinishReasonStop:        openai.FinishReasonStop,
	genai.FinishReasonMaxTokens:   openai.FinishReasonLength,
	genai.FinishReasonSafety:      openai.FinishReasonContentFilter,
}

func toOpenAIFinishReason(reason genai.FinishReason) openai.FinishReason {
	val, ok := mapFinishReason[reason]
	if ok {
		return val
	}
	return openai.FinishReason(fmt.Sprintf("FinishReason(%s)", val))
}
