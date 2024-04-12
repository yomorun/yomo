package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/internal/oai"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
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
	ctx context.Context,
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

func TestParseZipperAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected string
	}{
		{
			name:     "Valid address",
			addr:     "192.168.1.100:9000",
			expected: "192.168.1.100:9000",
		},
		{
			name:     "Valid address of localhost",
			addr:     "localhost",
			expected: "localhost:9000",
		},

		{
			name:     "Invalid address",
			addr:     "invalid",
			expected: DefaultZipperAddr,
		},
		{
			name:     "Localhost",
			addr:     "localhost:9000",
			expected: "localhost:9000",
		},
		{
			name:     "Unspecified IP",
			addr:     "0.0.0.0:9000",
			expected: "127.0.0.1:9000", // Expect the local IP
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseZipperAddr(tt.addr)
			assert.Equal(t, tt.expected, got, tt.name)
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		conf        map[string]interface{}
		expectError bool
		expected    *Config
	}{
		{
			name:        "Config not found",
			conf:        map[string]interface{}{},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Config format error",
			conf: map[string]interface{}{
				"ai": "invalid",
			},
			expectError: true,
			expected:    nil,
		},
		{
			name: "Valid config",
			conf: map[string]interface{}{
				"ai": map[string]interface{}{
					"server": map[string]interface{}{
						"addr": "localhost:9000",
					},
				},
			},
			expectError: false,
			expected: &Config{
				Server: Server{
					Addr: "localhost:9000",
				},
			},
		},
		{
			name: "Default server address",
			conf: map[string]interface{}{
				"ai": map[string]interface{}{
					"server": map[string]interface{}{},
				},
			},
			expectError: false,
			expected: &Config{
				Server: Server{
					Addr: ":8000",
				},
			},
		},
		{
			name: "malformaled config",
			conf: map[string]interface{}{
				"hello": "world",
			},
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.conf)
			if err != nil {
				assert.Equal(t, tt.expectError, true, tt.name)
			} else {
				assert.Equal(t, tt.expected, got, tt.name)
			}
		})
	}
}

type MockLLMProvider struct {
	name string
}

var _ LLMProvider = &MockLLMProvider{}

func (m *MockLLMProvider) Name() string {
	return m.name
}

func (m *MockLLMProvider) GetChatCompletions(chatCompletionRequest *ai.ChatCompletionRequest) (*ai.ChatCompletionResponse, error) {
	return nil, nil
}

func TestListProviders(t *testing.T) {
	t.Cleanup(func() {
		providers = sync.Map{}
		defaultProvider = nil
	})
	providers.Store("provider1", &MockLLMProvider{})
	providers.Store("provider2", &MockLLMProvider{})
	providers.Store("provider3", &MockLLMProvider{})

	expected := []string{"provider1", "provider2", "provider3"} // Replace with the expected provider names

	val := ListProviders()

	assert.ElementsMatch(t, expected, val)
}

func TestRegisterProvider(t *testing.T) {
	t.Cleanup(func() {
		providers = sync.Map{}
		defaultProvider = nil
	})
	provider := &MockLLMProvider{name: "testProvider"}

	// Test registering a provider
	RegisterProvider(provider)

	// Check if the provider was registered
	storedProvider, ok := providers.Load(provider.Name())
	assert.True(t, ok)
	assert.Equal(t, provider, storedProvider)
}

func TestSetDefaultProvider(t *testing.T) {
	t.Cleanup(func() {
		providers = sync.Map{}
		defaultProvider = nil
	})
	provider := &MockLLMProvider{name: "testProvider"}

	// Register a provider
	RegisterProvider(provider)

	// Set the provider as default
	SetDefaultProvider(provider.Name())

	// Check if the default provider was set
	assert.Equal(t, provider, defaultProvider)
}

func TestGetProviderAndSetDefault(t *testing.T) {
	t.Cleanup(func() {
		providers = sync.Map{}
		defaultProvider = nil
	})
	provider := &MockLLMProvider{name: "testProvider"}

	// Register a provider
	RegisterProvider(provider)

	// Get the provider and set it as default
	retrievedProvider, err := GetProviderAndSetDefault(provider.Name())

	// Check if the correct provider was retrieved and set as default
	assert.NoError(t, err)
	assert.Equal(t, provider, retrievedProvider)
	assert.Equal(t, provider, defaultProvider)
}

func TestGetDefaultProvider(t *testing.T) {
	t.Cleanup(func() {
		providers = sync.Map{}
		defaultProvider = nil
	})

	provider1 := &MockLLMProvider{name: "provider1"}
	provider2 := &MockLLMProvider{name: "provider2"}

	// Register first provider
	RegisterProvider(provider1)

	// Test getting the default provider when none is set
	// The first available provider should be returned
	p, err := GetDefaultProvider()
	assert.NoError(t, err)
	assert.Equal(t, provider1, p)

	// Register second provider
	RegisterProvider(provider2)

	// Set the second provider as default
	SetDefaultProvider(provider2.Name())

	// Test getting the default provider when one is set
	// The default provider should be returned
	p, err = GetDefaultProvider()
	assert.NoError(t, err)
	assert.Equal(t, provider2, p)
}

func TestServiceContext(t *testing.T) {
	// Create a new service
	service := &Service{}

	// Create a new context
	ctx := context.Background()

	// Add the service to the context
	ctx = WithServiceContext(ctx, service)

	// Retrieve the service from the context
	retrievedService := FromServiceContext(ctx)

	// Check if the correct service was retrieved
	assert.Equal(t, service, retrievedService)

	// Test with a context that does not contain a service
	ctx = context.Background()
	retrievedService = FromServiceContext(ctx)

	// Check if no service was retrieved
	assert.Nil(t, retrievedService)
}

func TestHandleOverview(t *testing.T) {
	functionDefinition := &ai.FunctionDefinition{
		Name:        "function1",
		Description: "desc1",
		Parameters: &ai.FunctionParameters{
			Type: "type1",
			Properties: map[string]*ai.ParameterProperty{
				"prop1": {Type: "type1", Description: "desc1"},
				"prop2": {Type: "type2", Description: "desc2"},
			},
			Required: []string{"prop1"},
		},
	}
	r := register.GetRegister()
	r.RegisterFunction(100, functionDefinition, 200, nil)

	register.SetRegister(r)

	// Create a new mock service
	service := &Service{
		LLMProvider: &MockLLMProvider{},
	}

	// Create a new request
	req, err := http.NewRequest("GET", "/overview", nil)
	assert.NoError(t, err)

	// Add the service to the request context
	req = req.WithContext(WithServiceContext(req.Context(), service))

	// Record the response
	rr := httptest.NewRecorder()

	// Create a handler function
	handler := http.HandlerFunc(HandleOverview)

	// Serve the request
	handler.ServeHTTP(rr, req)

	// Check the response status code
	assert.Equal(t, http.StatusOK, rr.Code)

	// Check the response body
	// This is a basic check for an empty body, replace with your own logic
	assert.Equal(t, "{\"Functions\":{\"100\":{\"name\":\"function1\",\"description\":\"desc1\",\"parameters\":{\"type\":\"type1\",\"properties\":{\"prop1\":{\"type\":\"type1\",\"description\":\"desc1\"},\"prop2\":{\"type\":\"type2\",\"description\":\"desc2\"}},\"required\":[\"prop1\"]}}}}\n", rr.Body.String())
}

func TestNewBasicAPIServer(t *testing.T) {
	// Create a new mock provider
	provider := &MockLLMProvider{name: "testProvider"}

	// Create a new config
	config := &Config{}

	// Call the NewBasicAPIServer function
	server, err := NewBasicAPIServer("testServer", config, "localhost:8080", provider, "testCredential")

	// Check if no error was returned
	assert.NoError(t, err)

	// Check if the server was correctly created
	assert.Equal(t, "testServer", server.Name)
	assert.Equal(t, config, server.Config)
	assert.Equal(t, "localhost:8080", server.ZipperAddr)
	assert.Equal(t, provider, server.Provider)
	assert.Equal(t, "testCredential", server.serviceCredential)
}
