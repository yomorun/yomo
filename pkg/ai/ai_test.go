package ai

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/ai"
	_ "github.com/yomorun/yomo/pkg/ai/azopenai"
)

func TestAIServer(t *testing.T) {
	var err error
	go func() {
		err = ai.Serve()
	}()
	time.Sleep(300 * time.Millisecond)
	assert.NoError(t, err)
}

func TestGetChatCompletions(t *testing.T) {
	go ai.Serve()
	functionDefinition := `{"name":"get_current_weather","description":"Get the current weather in a given location","parameters":{"type":"object","properties":{"location":{"type":"string","description":"The city and state, e.g. San Francisco, CA"}},"required":["location"]}}`
	err := ai.RegisterFunction("test", 1, functionDefinition)
	functionDefinition2 := `{"name":"get_weather","description":"Get the current weather in a given location","parameters":{"type":"object","properties":{"location":{"type":"string","description":"The city and state, e.g. San Francisco, CA"}},"required":["location"]}}`
	err = ai.RegisterFunction("test", 1, functionDefinition2)
	assert.NoError(t, err)
	tools, err := ai.ListToolCalls("test", 1)
	assert.NoError(t, err)
	assert.NotEmpty(t, tools)
	for i, tool := range tools {
		jsonStr, err := json.Marshal(tool)
		assert.NoError(t, err)
		t.Logf("tool[%d]: %s\n", i, string(jsonStr))
	}
}
