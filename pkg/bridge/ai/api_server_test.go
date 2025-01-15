package ai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

func TestServer(t *testing.T) {
	// register a function definition to the register
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
	register.SetRegister(register.NewDefault())
	register.RegisterFunction(100, functionDefinition, 200, nil)

	// mock the provider and the req/res of the caller
	pd, err := provider.NewMock("mock provider", provider.MockChatCompletionResponse(stopResp, stopResp))
	if err != nil {
		t.Fatal(err)
	}

	flow := newMockDataFlow(newHandler(2 * time.Hour).handle)

	newCaller := func(_ yomo.Source, _ yomo.StreamFunction, _ metadata.M, _ time.Duration) (*Caller, error) {
		return mockCaller(nil), err
	}

	service := newService(pd, newCaller, &ServiceOptions{
		SourceBuilder:     func() yomo.Source { return flow },
		ReducerBuilder:    func() yomo.StreamFunction { return flow },
		MetadataExchanger: func(_ string) (metadata.M, error) { return metadata.M{"hello": "llm bridge"}, nil },
	})

	handler := DecorateHandler(NewServeMux(service), decorateReqContext(service, service.logger))

	// create a test server
	server := httptest.NewServer(handler)

	httpClient := server.Client()

	t.Run("GET /overview", func(t *testing.T) {
		url := fmt.Sprintf("%s/overview", server.URL)

		resp, err := httpClient.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, `{"Functions":{"100":{"name":"function1","description":"desc1","parameters":{"type":"type1","properties":{"prop1":{"type":"type1","description":"desc1"},"prop2":{"type":"type2","description":"desc2"}},"required":["prop1"]}}}}
`, string(body))
	})

	t.Run("POST /invoke", func(t *testing.T) {
		url := fmt.Sprintf("%s/invoke", server.URL)

		resp, err := httpClient.Post(url, "application/json", bytes.NewBufferString(`{"prompt": "Hi, How are you"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "{\"content\":\"Hello! I'm just a computer program, so I don't have feelings, but thanks for asking. How can I assist you today?\",\"finish_reason\":\"stop\",\"token_usage\":{\"prompt_tokens\":13,\"completion_tokens\":26}}\n", string(body))
	})

	t.Run("POST /v1/chat/completions", func(t *testing.T) {
		url := fmt.Sprintf("%s/v1/chat/completions", server.URL)

		resp, err := httpClient.Post(url, "application/json", bytes.NewBufferString(`{"messages":[{"role":"user","content":"Hi, How are you"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "{\"id\":\"chatcmpl-9blYknv9rHvr2dvCQKMeW21hlBpCX\",\"object\":\"chat.completion\",\"created\":1718787982,\"model\":\"gpt-4o-2024-05-13\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Hello! I'm just a computer program, so I don't have feelings, but thanks for asking. How can I assist you today?\"},\"finish_reason\":\"stop\",\"content_filter_results\":{\"hate\":{\"filtered\":false},\"self_harm\":{\"filtered\":false},\"sexual\":{\"filtered\":false},\"violence\":{\"filtered\":false},\"jailbreak\":{\"filtered\":false,\"detected\":false},\"profanity\":{\"filtered\":false,\"detected\":false}}}],\"usage\":{\"prompt_tokens\":13,\"completion_tokens\":26,\"total_tokens\":39,\"prompt_tokens_details\":null,\"completion_tokens_details\":null},\"system_fingerprint\":\"fp_f4e629d0a5\"}\n", string(body))
	})

	t.Run("illegal request", func(t *testing.T) {
		url := fmt.Sprintf("%s/v1/chat/completions", server.URL)

		resp, err := httpClient.Post(url, "application/json", bytes.NewBufferString(`some illegal request`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "{\"error\":{\"code\":\"400\",\"message\":\"invalid character 's' looking for beginning of value\"}}", string(body))
	})
}
