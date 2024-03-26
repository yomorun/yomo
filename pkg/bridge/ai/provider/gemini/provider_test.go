package gemini

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
)

func TestGeminiProvider_Name(t *testing.T) {
	provider := &Provider{}
	name := provider.Name()

	assert.Equal(t, "gemini", name)
}

func TestGeminiProvider_getApiUrl(t *testing.T) {
	provider := &Provider{
		APIKey: "test-api-key",
	}
	expected := "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=test-api-key"
	result := provider.getAPIURL()

	assert.Equal(t, expected, result)
}

func TestNewProvider(t *testing.T) {
	apiKey := "test-api-key"
	provider := NewProvider(apiKey)

	assert.Equal(t, apiKey, provider.APIKey)
}

func TestNewProviderWithEnvVar(t *testing.T) {
	expectedAPIKey := "test-api-key"
	os.Setenv("GEMINI_API_KEY", expectedAPIKey)
	provider := NewProvider("")

	assert.Equal(t, expectedAPIKey, provider.APIKey)
}

func TestNewProviderWithoutEnvVar(t *testing.T) {
	os.Unsetenv("GEMINI_API_KEY")
	provider := NewProvider("")

	assert.NotNil(t, provider.APIKey)
}

func TestGeminiProvider_prepareRequest(t *testing.T) {
	provider := &Provider{}

	userInstruction := "test instruction"
	tcs := map[uint32]ai.ToolCall{
		0: {Function: &ai.FunctionDefinition{Name: "function"}},
		1: {Function: &ai.FunctionDefinition{Name: "function"}},
	}

	body, toolCalls := provider.prepareRequest(userInstruction, tcs)

	assert.Equal(t, "user", body.Contents.Role)
	assert.Equal(t, userInstruction, body.Contents.Parts.Text)
	assert.Equal(t, len(tcs), len(toolCalls))

	for i, tc := range tcs {
		assert.Equal(t, convertStandardToFunctionDeclaration(tc.Function), toolCalls[i])
	}
}
