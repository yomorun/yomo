package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

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
	register.RegisterFunction(100, functionDefinition, 200, nil)

	// Create a new request
	req, err := http.NewRequest("GET", "/overview", nil)
	assert.NoError(t, err)

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
