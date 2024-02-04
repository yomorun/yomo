package ai

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/pkg/bridge/ai"
	_ "github.com/yomorun/yomo/pkg/bridge/ai/provider/azopenai"
)

func TestAIServer(t *testing.T) {
	var err error
	go func() {
		err = startAIServer()
	}()
	time.Sleep(300 * time.Millisecond)
	assert.NoError(t, err)
}

func TestGetChatCompletions(t *testing.T) {
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
	aiConfig := ai.Config{
		Server: ai.Server{
			Addr: "localhost:6000",
			Endpoints: map[string]string{
				"chat_completions": "/chat/completions",
			},
			Credential: "",
			Provider:   "azopenai",
		},
		Providers: map[string]ai.Provider{},
	}
	confData, _ := yaml.Marshal(map[string]ai.Config{"ai": aiConfig})
	var mapConf map[string]any
	yaml.Unmarshal(confData, &mapConf)
	return ai.Serve(mapConf, "localhost:9000")
}
