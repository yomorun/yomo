package ai

import (
	"fmt"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

func TestRegisterFunction(t *testing.T) {
	r := register.NewDefault()
	connHandler := registerFunction(r)(func(c *core.Connection) {})

	t.Run("source", func(t *testing.T) {
		conn := mockSourceConn(1, "source")
		connHandler(conn)

		toolCalls, _ := r.ListToolCalls(conn.Metadata())
		assert.Equal(t, []openai.Tool{}, toolCalls)
	})

	t.Run("stream function", func(t *testing.T) {
		conn := mockSfnConn(2, "sfn")
		connHandler(conn)

		toolCalls, _ := r.ListToolCalls(conn.Metadata())

		want := []openai.Tool{
			{Type: "function", Function: &openai.FunctionDefinition{Name: "sfn"}},
		}

		assert.Equal(t, want, toolCalls)
	})
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

func mockSfnConn(id uint64, name string) *core.Connection {
	md := metadata.M{
		ai.FunctionDefinitionKey: fmt.Sprintf(`{"name": "%s"}`, name),
	}
	return core.NewConnection(id, name, "mock-sfn-id", core.ClientTypeStreamFunction, md, []frame.Tag{0x33}, nil, ylog.Default())
}

func mockSourceConn(id uint64, name string) *core.Connection {
	return core.NewConnection(id, name, "mock-source-id", core.ClientTypeSource, metadata.New(), []frame.Tag{0x33}, nil, ylog.Default())
}
