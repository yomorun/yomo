package azaifoundry

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestAzureAIFoundryProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "azaifoundry", name)
}

func TestAzureOpenAIProvider_GetChatCompletions(t *testing.T) {
	config := newConfig("test", "https://yomo.openai.azure.com", "test-version")
	config.HTTPClient = &http.Client{Timeout: time.Millisecond}

	provider := &Provider{
		APIKey:      "test",
		APIEndpoint: "https://yomo.openai.azure.com",
		APIVersion:  "test-version",
		Model:       "deepseek-v3-0324",
		client:      openai.NewClientWithConfig(config),
	}
	msgs := []openai.ChatCompletionMessage{
		{
			Role:    "user",
			Content: "hello",
		},
		{
			Role:    "system",
			Content: "I'm a bot",
		},
	}
	req := openai.ChatCompletionRequest{
		Model:    "deepseek-v3-0324",
		Messages: msgs,
	}

	_, err := provider.GetChatCompletions(context.TODO(), req, nil)
	assert.NotNil(t, err, "Expected timeout error, but got nil")

	_, err = provider.GetChatCompletionsStream(context.TODO(), req, nil)
	assert.NotNil(t, err, "Expected timeout error, but got nil")
}

func TestNewProvider(t *testing.T) {
	// Test case parameters
	apiEndpoint := "https://test.openai.azure.com"
	apiKey := "test-api-key"
	apiVersion := "2023-05-15"
	model := "test-model"

	// Create the provider
	provider := NewProvider(apiEndpoint, apiKey, apiVersion, model)

	// Verify all fields are set correctly
	assert.Equal(t, apiKey, provider.APIKey)
	assert.Equal(t, apiEndpoint, provider.APIEndpoint)
	assert.Equal(t, model, provider.Model)
	assert.Equal(t, apiVersion, provider.APIVersion)
	assert.NotNil(t, provider.client)
}

func TestNewConfigFunction(t *testing.T) {
	// Test parameters
	apiKey := "test-key"
	apiEndpoint := "https://test-endpoint.com"
	apiVersion := "2023-12-01"

	// Call the function
	config := newConfig(apiKey, apiEndpoint, apiVersion)

	// Verify results
	assert.Equal(t, apiEndpoint+"/models/", config.BaseURL)
	assert.Equal(t, apiVersion, config.APIVersion)
}
