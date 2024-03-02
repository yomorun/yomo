package openai

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
)

func TestOpenAIProvider_RegisterFunction(t *testing.T) {
	fns = sync.Map{}
	provider := &OpenAIProvider{}
	tag := uint32(66)
	functionDefinition := &ai.FunctionDefinition{
		Name: "TestFunction",
	}
	connID := uint64(88)

	err := provider.RegisterFunction(tag, functionDefinition, connID)
	assert.NoError(t, err)

	fn, ok := fns.Load(connID)
	assert.True(t, ok)
	assert.Equal(t, connID, fn.(*connectedFn).connID)
	assert.Equal(t, tag, fn.(*connectedFn).tag)
	assert.Equal(t, "function", fn.(*connectedFn).tc.Type)
	assert.Equal(t, functionDefinition.Name, fn.(*connectedFn).tc.Function.Name)
}

func TestOpenAIProvider_UnregisterFunction(t *testing.T) {
	provider := &OpenAIProvider{}
	connID := uint64(1)

	// Assuming a function is already registered with connID
	err := provider.UnregisterFunction("", connID)
	assert.NoError(t, err)

	_, ok := fns.Load(connID)
	assert.False(t, ok)
}

func TestOpenAIProvider_ListToolCalls(t *testing.T) {
	provider := &OpenAIProvider{}

	// Assuming some functions are already registered
	toolCalls, err := provider.ListToolCalls()
	assert.NoError(t, err)

	// Replace with your own checks
	assert.NotEmpty(t, toolCalls)
}

func TestOpenAIProvider_GetOverview(t *testing.T) {
	provider := &OpenAIProvider{}

	// Assuming some functions are already registered
	overview, err := provider.GetOverview()
	assert.NoError(t, err)

	// Replace with your own checks
	assert.NotEmpty(t, overview.Functions)
}

func TestHasToolCalls(t *testing.T) {
	// Assuming some functions are already registered
	toolCalls, hasCalls := hasToolCalls()

	// Replace with your own checks
	assert.True(t, hasCalls)
	assert.NotEmpty(t, toolCalls)
}

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
