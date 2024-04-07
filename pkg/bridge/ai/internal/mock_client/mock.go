package mock_client

import (
	"context"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
)

// MockOpenAIClient is a mock implementation of the OpenAIClient for test
type MockOpenAIClient struct {
	APIEndpoint     string
	AuthHeaderKey   string
	AuthHeaderValue string
	Request         *ai.ChatCompletionRequest
}

var _ oai.OpenAIRequester = &MockOpenAIClient{}

// ChatCompletion is a mock implementation of the ChatCompletion method
func (c *MockOpenAIClient) ChatCompletions(
	_ context.Context,
	apiEndpoint string,
	authHeaderKey string,
	authHeaderValue string,
	req *ai.ChatCompletionRequest,
) (*ai.ChatCompletionResponse, error) {
	c.APIEndpoint = apiEndpoint
	c.AuthHeaderKey = authHeaderKey
	c.AuthHeaderValue = authHeaderValue
	c.Request = req

	return nil, nil
}
