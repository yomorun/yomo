package gemini

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeminiProvider_Name(t *testing.T) {
	provider := &GeminiProvider{}
	name := provider.Name()

	assert.Equal(t, "gemini", name)
}

func TestGeminiProvider_getApiUrl(t *testing.T) {
	provider := &GeminiProvider{
		APIKey: "test-api-key",
	}
	expected := "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=test-api-key"
	result := provider.getApiUrl()

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
