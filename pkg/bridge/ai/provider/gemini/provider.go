// Package gemini is used to provide the gemini service
package gemini

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// Provider is the provider for google gemini.
type Provider struct {
	model  string
	client *openai.Client
}

var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new gemini provider.
func NewProvider(apiKey string) *Provider {
	c := openai.DefaultConfig(apiKey)
	c.BaseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"
	c.EmptyMessagesLimit = 300

	client := openai.NewClientWithConfig(c)

	return &Provider{
		model:  "gemini-2.0-flash",
		client: client,
	}
}

// GetChatCompletions implements provider.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (openai.ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	return p.client.CreateChatCompletion(ctx, req)
}

// GetChatCompletionsStream implements provider.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (provider.ResponseRecver, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	return p.client.CreateChatCompletionStream(ctx, req)
}

// Name implements provider.LLMProvider.
func (p *Provider) Name() string {
	return "gemini"
}
