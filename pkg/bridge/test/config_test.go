package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
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
			expected: pkgai.DefaultZipperAddr,
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
			got := pkgai.ParseZipperAddr(tt.addr)
			assert.Equal(t, tt.expected, got, tt.name)
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		conf        map[string]interface{}
		expectError bool
		expected    *pkgai.Config
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
			expected: &pkgai.Config{
				Server: pkgai.Server{
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
			expected: &pkgai.Config{
				Server: pkgai.Server{
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
			got, err := pkgai.ParseConfig(tt.conf)
			if err != nil {
				assert.Equal(t, tt.expectError, true, tt.name)
			} else {
				assert.Equal(t, tt.expected, got, tt.name)
			}
		})
	}
}
