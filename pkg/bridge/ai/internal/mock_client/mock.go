package mock_client

import (
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
)

// MockOpenAIClient is a mock implementation of the OpenAIClient for test
type MockOpenAIClient struct {
	APIEndpoint       string
	AuthHeaderKey     string
	AuthHeaderValue   string
	BaseRequestbody   oai.ReqBody
	BaseSystemMessage string
	UserInstruction   string
	ChainMessage      ai.ChainMessage
	Metadata          metadata.M
	IfWithTool        bool
}

var _ oai.ILLMClient = &MockOpenAIClient{}

// ChatCompletion is a mock implementation of the ChatCompletion method
func (c *MockOpenAIClient) ChatCompletion(apiEndpoint string, authHeaderKey string, authHeaderValue string, baseRequestbody oai.ReqBody, baseSystemMessage string, userInstruction string, chainMessage ai.ChainMessage, md metadata.M, ifWithTool bool) (*ai.InvokeResponse, error) {
	c.APIEndpoint = apiEndpoint
	c.AuthHeaderKey = authHeaderKey
	c.AuthHeaderValue = authHeaderValue
	c.BaseRequestbody = baseRequestbody
	c.BaseSystemMessage = baseSystemMessage
	c.UserInstruction = userInstruction
	c.ChainMessage = chainMessage
	c.Metadata = md
	c.IfWithTool = ifWithTool

	return nil, nil
}
