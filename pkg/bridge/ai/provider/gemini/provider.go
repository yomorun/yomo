// Package gemini is used to provide the gemini service
package gemini

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/google/generative-ai-go/genai"
	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/id"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Provider is the provider for google gemini.
type Provider struct {
	apiKey string
	model  string
	client *genai.Client
}

var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new gemini provider.
func NewProvider(apiKey string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatal("new gemini client: ", err)
	}

	return &Provider{
		apiKey: apiKey,
		model:  "gemini-1.5-pro-latest",
		client: client,
	}
}

func (p *Provider) generativeModel(req openai.ChatCompletionRequest) *genai.GenerativeModel {
	model := p.client.GenerativeModel(p.model)

	model.SetTemperature(req.Temperature)
	model.SetTopP(req.TopP)
	if req.MaxTokens > 0 {
		model.SetMaxOutputTokens(int32(req.MaxTokens))
	}
	if len(req.Stop) != 0 {
		model.StopSequences = req.Stop
	}

	return model
}

// GetChatCompletions implements provider.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (openai.ChatCompletionResponse, error) {
	model := p.generativeModel(req)

	chat := model.StartChat()

	parts := convertPart(chat, req, model, md)

	resp, err := chat.SendMessage(ctx, parts...)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}

	return convertToResponse(resp, p.model), nil
}

// GetChatCompletionsStream implements provider.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, md metadata.M) (provider.ResponseRecver, error) {
	model := p.generativeModel(req)

	chat := model.StartChat()

	parts := convertPart(chat, req, model, md)

	resp := chat.SendMessageStream(ctx, parts...)

	includeUsage := false
	if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
		includeUsage = true
	}

	recver := &recver{
		model:        p.model,
		underlying:   resp,
		includeUsage: includeUsage,
	}

	return recver, nil
}

// Name implements provider.LLMProvider.
func (p *Provider) Name() string {
	return "gemini"
}

type recver struct {
	done         bool
	id           string
	includeUsage bool
	usage        *openai.Usage
	model        string
	underlying   *genai.GenerateContentResponseIterator
}

// Recv implements provider.ResponseRecver.
func (r *recver) Recv() (response openai.ChatCompletionStreamResponse, err error) {
	if r.done {
		return openai.ChatCompletionStreamResponse{}, io.EOF
	}
	if r.id == "" {
		r.id = "chatcmpl-" + id.New(29)
	}
	if r.usage == nil {
		r.usage = &openai.Usage{}
	}
	resp, err := r.underlying.Next()
	if err == iterator.Done {
		r.usage.TotalTokens = r.usage.PromptTokens + r.usage.CompletionTokens
		usageResp := openai.ChatCompletionStreamResponse{
			ID:      r.id,
			Model:   r.model,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Usage:   r.usage,
			Choices: make([]openai.ChatCompletionStreamChoice, 0),
		}
		r.done = true
		return usageResp, nil
	}
	if err != nil {
		return openai.ChatCompletionStreamResponse{}, err
	}

	return convertToStreamResponse(r.id, resp, r.model, r.usage), nil
}
