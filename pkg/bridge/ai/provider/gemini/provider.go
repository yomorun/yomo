// Package gemini is used to provide the gemini service
package gemini

import (
	"context"
	"log"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

// Provider is the provider for google gemini.
type Provider struct {
	model  string
	client *openai.Client
}

var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new gemini provider.
func NewProvider(apiKey string) *Provider {
	httpClient, _, err := transport.NewHTTPClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatalln("gemini new http client: ", err)
	}

	client := openai.NewClientWithConfig(openai.ClientConfig{
		BaseURL:            "https://generativelanguage.googleapis.com/v1beta/openai/",
		HTTPClient:         httpClient,
		EmptyMessagesLimit: 300,
	})

	return &Provider{
		model:  "gemini-1.5-pro-latest",
		client: client,
	}
}

// GetChatCompletions implements provider.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (openai.ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	} else {
		req.Model = "google/" + req.Model
	}
	return p.client.CreateChatCompletion(ctx, req)
}

// GetChatCompletionsStream implements provider.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (provider.ResponseRecver, error) {
	if req.Model == "" {
		req.Model = p.model
	} else {
		req.Model = "google/" + req.Model
	}

	return p.client.CreateChatCompletionStream(ctx, req)
}

// Name implements provider.LLMProvider.
func (p *Provider) Name() string {
	return "gemini"
}
