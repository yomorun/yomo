package oai

import (
	"context"

	"github.com/yomorun/yomo/ai"
)

// OpenAIRequester is the interface for OpenAI API client
type OpenAIRequester interface {
	// ChatCompletions is the method to get chat completions
	ChatCompletions(ctx context.Context, apiEndpoint string, authHeaderKey string, authHeaderValue string, chatCompletionRequest *ai.ChatCompletionRequest) (*ai.ChatCompletionResponse, error)
}
