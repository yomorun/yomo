// Package vertexai is used to provide the vertexai service
package vertexai

import (
	"context"
	"fmt"
	"log"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

// Provider is the provider for google vertexai.
type Provider struct {
	model  string
	client *openai.Client
}

var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new vertexai provider.
func NewProvider(projectID, location, model, credentialsFile string) *Provider {
	httpClient, _, err := transport.NewHTTPClient(
		context.Background(),
		option.WithScopes("https://www.googleapis.com/auth/cloud-platform"),
		option.WithCredentialsFile(credentialsFile),
	)
	if err != nil {
		log.Fatalln("vertexai new http client: ", err)
	}

	// if location is "global"
	baseURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/endpoints/openapi", location, projectID, location)
	if location == "global" {
		baseURL = fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/endpoints/openapi", projectID, location)
	}

	client := openai.NewClientWithConfig(openai.ClientConfig{
		BaseURL:            baseURL,
		HTTPClient:         httpClient,
		EmptyMessagesLimit: 300,
	})

	if model == "" {
		model = "gemini-2.5-flash"
	}

	model = "google/" + model

	return &Provider{
		model:  model,
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
	return "vertexai"
}
