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

	for i, candidate := range in.Candidates {
		for j, part := range candidate.Content.Parts {
			index := i + j
			switch pp := part.(type) {
			case genai.Text:
				out.Choices = append(out.Choices, openai.ChatCompletionChoice{
					Index: index,
					Message: openai.ChatCompletionMessage{
						Content: string(pp),
						Role:    openai.ChatMessageRoleUser,
					},
					FinishReason: toOpenAIFinishReason(candidate.FinishReason),
				})
			case genai.FunctionCall:
				args, _ := json.Marshal(pp.Args)

				out.Choices = append(out.Choices, openai.ChatCompletionChoice{
					Index: index,
					Message: openai.ChatCompletionMessage{
						Role: openai.ChatMessageRoleAssistant,
						ToolCalls: []openai.ToolCall{{
							Index:    genai.Ptr(index),
							ID:       pp.Name + "-" + id.New(4),
							Type:     openai.ToolTypeFunction,
							Function: openai.FunctionCall{Name: pp.Name, Arguments: string(args)},
						}},
					},
					FinishReason: openai.FinishReasonToolCalls,
				})
			}
		}
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

	for i, candidate := range in.Candidates {
		parts := candidate.Content.Parts

		for j, part := range parts {
			index := i + j
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

				out.Choices = append(out.Choices, openai.ChatCompletionStreamChoice{
					Index: index,
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Role: openai.ChatMessageRoleAssistant,
						ToolCalls: []openai.ToolCall{
							{
								Index:    genai.Ptr(index),
								ID:       pp.Name,
								Type:     openai.ToolTypeFunction,
								Function: openai.FunctionCall{Name: pp.Name, Arguments: string(args)},
							},
						},
					},
					FinishReason: toOpenAIFinishReason(candidate.FinishReason),
				})
			}
		}
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
