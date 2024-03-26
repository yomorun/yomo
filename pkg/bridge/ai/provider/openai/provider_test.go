package openai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/mock_client"
)

func TestNewProvider(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("OPENAI_API_KEY", "test_api_key")
	os.Setenv("OPENAI_MODEL", "test_model")

	provider := NewProvider("", "")

	assert.Equal(t, "test_api_key", provider.APIKey)
	assert.Equal(t, "test_model", provider.Model)
}

func TestOpenAIProvider_Name(t *testing.T) {
	provider := &OpenAIProvider{}

	name := provider.Name()

	assert.Equal(t, "openai", name)
}

func TestCloudflareOpenAIProvider_GetChatCompletions(t *testing.T) {
	client := &mock_client.MockOpenAIClient{}

	provider := &OpenAIProvider{
		APIKey: "test",
		Model:  "test",
		client: client,
	}

	provider.GetChatCompletions("user", "system", ai.ChainMessage{}, nil, false)

	assert.Equal(t, "test", client.BaseRequestbody.Model)
	assert.Equal(t, "user", client.UserInstruction)
	assert.Equal(t, "system", client.BaseSystemMessage)
	assert.Equal(t, false, client.IfWithTool)
}
