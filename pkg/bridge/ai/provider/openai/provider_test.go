package openai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
