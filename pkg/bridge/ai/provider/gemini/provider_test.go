package gemini

import (
	"os"
	"testing"
)

func TestGeminiProvider_Name(t *testing.T) {
	provider := &GeminiProvider{}

	name := provider.Name()

	if name != "gemini" {
		t.Errorf("Name() = %v, want %v", name, "gemini")
	}
}

func TestGeminiProvider_getApiUrl(t *testing.T) {
	provider := &GeminiProvider{
		APIKey: "test-api-key",
	}

	expected := "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=test-api-key"

	result := provider.getApiUrl()

	if result != expected {
		t.Errorf("getApiUrl() = %v, want %v", result, expected)
	}
}

func TestNewProvider(t *testing.T) {
	apiKey := "test-api-key"
	provider := NewProvider(apiKey)

	if provider.APIKey != apiKey {
		t.Errorf("NewProvider() = %v, want %v", provider.APIKey, apiKey)
	}
}

func TestNewProviderWithEnvVar(t *testing.T) {
	// Set up
	expectedAPIKey := "test-api-key"
	os.Setenv("GEMINI_API_KEY", expectedAPIKey)

	// Call the function under test
	provider := NewProvider("")

	// Check the result
	if provider.APIKey != expectedAPIKey {
		t.Errorf("NewProvider() = %v, want %v", provider.APIKey, expectedAPIKey)
	}
}

func TestNewProviderWithoutEnvVar(t *testing.T) {
	// Set up
	os.Unsetenv("GEMINI_API_KEY")

	// Call the function under test
	provider := NewProvider("")

	// Check the result
	if provider.APIKey != "" {
		t.Errorf("NewProvider() = %v, want %v", provider.APIKey, "")
	}
}
