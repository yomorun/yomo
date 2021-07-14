package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig("./mock/workflow.yaml")
	assert.Nil(t, err)
	// server
	assert.Equal(t, "Server", cfg.Name)
	assert.Equal(t, "127.0.0.1", cfg.Host)
	assert.Equal(t, 8111, cfg.Port)
	// functions
	assert.Equal(t, 3, len(cfg.Functions))
	fn := cfg.Functions[0]
	assert.Equal(t, "fun1", fn.Name)
}
