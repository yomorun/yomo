// Package azaifoundry is used to provide the Azure OpenAI service
package azaifoundry

import (
	"context"
	"strings"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"

	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
)

// Provider is the provider for Azure OpenAI
type Provider struct {
	APIKey      string
	APIEndpoint string
	APIVersion  string
	Model       string
	client      *openai.Client
}

var _ provider.LLMProvider = &Provider{}

// NewProvider creates a new Azure AI Foundry
func NewProvider(apiEndpoint string, apiKey string, apiVersion string, model string) *Provider {
	config := newConfig(apiKey, apiEndpoint, apiVersion)

	client := openai.NewClientWithConfig(config)

	return &Provider{
		APIKey:      apiKey,
		APIEndpoint: apiEndpoint,
		Model:       model,
		APIVersion:  apiVersion,
		client:      client,
	}
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "azaifoundry"
}

// GetChatCompletions get chat completions for ai service
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (openai.ChatCompletionResponse, error) {
	req.Model = p.Model
	return p.client.CreateChatCompletion(ctx, req)
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (provider.ResponseRecver, error) {
	req.Model = p.Model
	return p.client.CreateChatCompletionStream(ctx, req)
}

func newConfig(apiKey string, apiEndpoint string, apiVersion string) openai.ClientConfig {
	if !strings.HasSuffix(apiEndpoint, "/") {
		apiEndpoint += "/"
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = apiEndpoint + "models/"
	config.APIVersion = apiVersion

	return config
}

/** ps: I really want to change to openai-go **/
// c := openai.NewClient(
// 	withEndpoint(apiEndpoint, apiVersion),
// 	withAPIKey(apiKey),
// )

// func withEndpoint(endpoint string, apiVersion string) option.RequestOption {
// 	if !strings.HasSuffix(endpoint, "/") {
// 		endpoint += "/"
// 	}
// 	endpoint += "models/"
// 	withQueryAdd := option.WithQueryAdd("api-version", apiVersion)
// 	withEndpoint := option.WithBaseURL(endpoint)

// 	return requestconfig.RequestOptionFunc(func(rc *requestconfig.RequestConfig) error {
// 		if apiVersion == "" {
// 			return fmt.Errorf("apiVersion is an empty string, but needs to be set. See https://learn.microsoft.com/en-us/azure/ai-services/openai/reference#rest-api-versioning for details.")
// 		}
// 		if err := withQueryAdd.Apply(rc); err != nil {
// 			return err
// 		}
// 		if err := withEndpoint.Apply(rc); err != nil {
// 			return err
// 		}
// 		return nil
// 	})
// }

// func withAPIKey(apiKey string) option.RequestOption {
// 	return option.WithHeader("api-key", apiKey)
// }
