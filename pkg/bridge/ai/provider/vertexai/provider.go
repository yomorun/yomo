// Package vertexai is used to provide the vertexai service
package vertexai

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

// Provider is the provider for google vertexai.
type Provider struct {
	model       string
	client      *openai.Client
	thoughtSign *thoughtSignature
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

	var baseURL string
	if location == "global" {
		baseURL = fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/endpoints/openapi", projectID, location)
	} else {
		baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/endpoints/openapi", location, projectID, location)
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
		model:       model,
		client:      client,
		thoughtSign: newThoughtSignature(),
	}
}

// GetChatCompletions implements provider.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (openai.ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	} else {
		req.Model = "google/" + req.Model
	}

	p.thoughtSign.AttachExtra(req)

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}

	for _, choice := range resp.Choices {
		p.thoughtSign.SaveExtra(choice.Message.ToolCalls)
	}

	return resp, nil
}

// GetChatCompletionsStream implements provider.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (provider.ResponseRecver, error) {
	if req.Model == "" {
		req.Model = p.model
	} else {
		req.Model = "google/" + req.Model
	}
	p.thoughtSign.AttachExtra(req)

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}
	return &recver{resp: stream}, nil
}

// Name implements provider.LLMProvider.
func (p *Provider) Name() string {
	return "vertexai"
}

type recver struct {
	resp        *openai.ChatCompletionStream
	thoughtSign *thoughtSignature
	toolCalls   []openai.ToolCall
}

func (r *recver) Recv() (openai.ChatCompletionStreamResponse, error) {
	resp, err := r.resp.Recv()
	if err == io.EOF {
		r.thoughtSign.SaveExtra(r.toolCalls)
	}
	if err != nil {
		return openai.ChatCompletionStreamResponse{}, err
	}
	for _, choice := range resp.Choices {
		r.toolCalls = append(r.toolCalls, choice.Delta.ToolCalls...)
	}
	return resp, nil
}

type thoughtSignature struct {
	inner *expirable.LRU[string, map[string]any]
}

func newThoughtSignature() *thoughtSignature {
	return &thoughtSignature{
		inner: expirable.NewLRU[string, map[string]any](200, nil, time.Hour),
	}
}

func (k *thoughtSignature) SaveExtra(toolCalls []openai.ToolCall) {
	for _, toolCall := range toolCalls {
		if extra := toolCall.ExtraContent; extra != nil && extra["google"] != nil {
			k.inner.Add(toolCall.ID, extra)
		}
	}
}

func (k *thoughtSignature) AttachExtra(req openai.ChatCompletionRequest) {
	for i, msg := range req.Messages {
		for j, toolCall := range msg.ToolCalls {

			if extra, ok := k.inner.Get(toolCall.ID); ok {
				// k.inner.Remove(toolCall.ID)
				req.Messages[i].ToolCalls[j].ExtraContent = extra
			}
		}
	}
}
