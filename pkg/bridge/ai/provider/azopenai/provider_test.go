package azopenai

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
)

func TestNewProvider(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("AZURE_OPENAI_API_KEY", "test_api_key")
	os.Setenv("AZURE_OPENAI_API_ENDPOINT", "test_api_endpoint")
	os.Setenv("AZURE_OPENAI_DEPLOYMENT_ID", "test_deployment_id")
	os.Setenv("AZURE_OPENAI_API_VERSION", "test_api_version")

	provider := NewProvider("", "", "", "")

	assert.Equal(t, "test_api_key", provider.APIKey)
	assert.Equal(t, "test_api_endpoint", provider.APIEndpoint)
	assert.Equal(t, "test_deployment_id", provider.DeploymentID)
	assert.Equal(t, "test_api_version", provider.APIVersion)
}

func TestAzureOpenAIProvider_Name(t *testing.T) {
	provider := &AzureOpenAIProvider{}

	name := provider.Name()

	assert.Equal(t, "azopenai", name)
}

func TestAzureOpenAIProvider_RegisterFunction(t *testing.T) {
	fns = sync.Map{}
	provider := &AzureOpenAIProvider{}
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

func TestAzureOpenAIProvider_UnregisterFunction(t *testing.T) {
	provider := &AzureOpenAIProvider{}
	err := provider.UnregisterFunction("", 1)
	assert.NoError(t, err)
	_, ok := fns.Load(1)
	assert.False(t, ok)
}

func TestAzureOpenAIProvider_ListToolCalls(t *testing.T) {
	fns = sync.Map{}
	provider := &AzureOpenAIProvider{}

	// Add a connectedFn to fns for testing
	fns.Store(1, &connectedFn{
		tag: 0x16,
		tc: ai.ToolCall{
			Type: "function",
			Function: &ai.FunctionDefinition{
				Name: "TestFunction",
			},
		},
	})

	toolCalls, err := provider.ListToolCalls()

	assert.NoError(t, err)
	assert.NotNil(t, toolCalls[0x16])
	assert.Equal(t, toolCalls[0x16].Function.Name, "TestFunction")
}

func TestAzureOpenAIProvider_GetOverview(t *testing.T) {
	fns = sync.Map{}
	provider := &AzureOpenAIProvider{}

	// Add a connectedFn to fns for testing
	fns.Store(1, &connectedFn{
		tag: 0x16,
		tc: ai.ToolCall{Function: &ai.FunctionDefinition{
			Name: "TestFunction",
		}},
	})

	overview, err := provider.GetOverview()

	assert.NoError(t, err)
	assert.NotNil(t, overview)
	assert.NotNil(t, overview.Functions[0x16])
	assert.Equal(t, overview.Functions[0x16].Name, "TestFunction")
}
