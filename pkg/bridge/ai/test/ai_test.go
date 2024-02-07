package test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/azopenai"
)

func TestAIServer(t *testing.T) {
	var err error
	go func() {
		err = startAIServer()
	}()
	time.Sleep(300 * time.Millisecond)
	assert.NoError(t, err)
}

func TestAIToolCalls(t *testing.T) {
	go startAIServer()
	functionDefinition := `{"name":"get_current_weather","description":"Get the current weather in a given location","parameters":{"type":"object","properties":{"location":{"type":"string","description":"The city and state, e.g. San Francisco, CA"}},"required":["location"]}}`
	err := ai.RegisterFunction("test", 1, []byte(functionDefinition))
	functionDefinition2 := `{"name":"get_weather","description":"Get the current weather in a given location","parameters":{"type":"object","properties":{"location":{"type":"string","description":"The city and state, e.g. San Francisco, CA"}},"required":["location"]}}`
	err = ai.RegisterFunction("test", 1, []byte(functionDefinition2))
	assert.NoError(t, err)
	tools, err := ai.ListToolCalls("test")
	assert.NoError(t, err)
	assert.NotEmpty(t, tools)
	for i, tool := range tools {
		jsonStr, err := json.Marshal(tool)
		assert.NoError(t, err)
		t.Logf("tool[%d]: %s\n", i, string(jsonStr))
	}
}

func startAIServer() error {
	ai.RegisterProvider(azopenai.New())
	aiConfig := &ai.Config{
		Server: ai.Server{
			Addr: "localhost:6000",
			Endpoints: map[string]string{
				"chat_completions": "/chat/completions",
			},
			Provider: "azopenai",
		},
		Providers: map[string]ai.Provider{},
	}
	return ai.Serve(aiConfig, "localhost:9000")
}
