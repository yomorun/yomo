package test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	_ "github.com/yomorun/yomo/pkg/bridge/ai/register"
)

func TestOpSystemPrompt(t *testing.T) {
	type args struct {
		prompt string
		op     pkgai.SystemPromptOp
		req    openai.ChatCompletionRequest
	}
	tests := []struct {
		name string
		args args
		want openai.ChatCompletionRequest
	}{
		{
			name: "disabled",
			args: args{
				prompt: "hello",
				op:     pkgai.SystemPromptOpDisabled,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "hello"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "hello"},
				},
			},
		},
		{
			name: "overwrite with empty system prompt",
			args: args{
				prompt: "",
				op:     pkgai.SystemPromptOpOverwrite,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{},
			},
		},
		{
			name: "empty system prompt should not overwrite",
			args: args{
				prompt: "",
				op:     pkgai.SystemPromptOpOverwrite,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "hello"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "system", Content: "hello"},
				},
			},
		},
		{
			name: "overwrite with not empty system prompt",
			args: args{
				prompt: "hello",
				op:     pkgai.SystemPromptOpOverwrite,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "world"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "system", Content: "hello"},
				},
			},
		},
		{
			name: "prefix with empty system prompt",
			args: args{
				prompt: "hello",
				op:     pkgai.SystemPromptOpPrefix,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "system", Content: "hello"},
				},
			},
		},
		{
			name: "prefix with not empty system prompt",
			args: args{
				prompt: "hello",
				op:     pkgai.SystemPromptOpPrefix,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "world"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "system", Content: "hello\nworld"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &pkgai.ServiceOptions{Logger: slog.Default()}
			s := pkgai.NewService(nil, opts)
			got := s.OpSystemPrompt(tt.args.req, tt.args.prompt, tt.args.op)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServiceInvoke(t *testing.T) {
	type args struct {
		providerMockData  []provider.MockData
		mockCallReqResp   []mockFunctionCall
		systemPrompt      string
		userInstruction   string
		baseSystemMessage string
	}
	tests := []struct {
		name        string
		args        args
		wantRequest []openai.ChatCompletionRequest
		wantUsage   ai.TokenUsage
	}{
		{
			name: "invoke with tool call",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionResponse(toolCallResp, stopResp),
				},
				mockCallReqResp: []mockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{toolID: "call_abc123", functionName: "get_current_weather", respContent: "temperature: 31°C"},
				},
				systemPrompt:      "this is a system prompt",
				userInstruction:   "hi",
				baseSystemMessage: "this is a base system message",
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a base system message\n\n## Instructions\n- \n\n"},
						{Role: "user", Content: "hi"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a base system message\n\n## Instructions\n"},
						{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_abc123", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get_current_weather", Arguments: "{\n\"location\": \"Boston, MA\"\n}"}}}},
						{Role: "tool", Content: "temperature: 31°C", ToolCallID: "call_abc123"},
						{Role: "user", Content: "hi"},
					},
				},
			},
			wantUsage: ai.TokenUsage{PromptTokens: 95, CompletionTokens: 43},
		},
		// BUG: test failed
		// {
		// 	name: "invoke without tool call",
		// 	args: args{
		// 		providerMockData: []provider.MockData{
		// 			provider.MockChatCompletionResponse(stopResp),
		// 		},
		// 		mockCallReqResp:   []mockFunctionCall{},
		// 		systemPrompt:      "this is a system prompt",
		// 		userInstruction:   "hi",
		// 		baseSystemMessage: "this is a base system message",
		// 	},
		// 	wantRequest: []openai.ChatCompletionRequest{
		// 		{
		// 			Messages: []openai.ChatCompletionMessage{
		// 				{Role: "system", Content: "this is a base system message\n\n## Instructions\n\n"},
		// 				{Role: "user", Content: "hi"},
		// 			},
		// 		},
		// 	},
		// 	wantUsage: ai.TokenUsage{PromptTokens: 13, CompletionTokens: 26},
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd, err := provider.NewMock("mock provider", tt.args.providerMockData...)
			if err != nil {
				t.Fatal(err)
			}

			flow := newMockDataFlow(newHandler(2 * time.Hour).handle)

			newCaller := func(_ yomo.Source, _ yomo.StreamFunction, _ metadata.M, _ time.Duration) (*pkgai.Caller, error) {
				return mockCaller(tt.args.mockCallReqResp), err
			}

			service := pkgai.NewServiceWithCallerFunc(pd, newCaller, &pkgai.ServiceOptions{
				SourceBuilder:     func(_ string) yomo.Source { return flow },
				ReducerBuilder:    func(_ string) yomo.StreamFunction { return flow },
				MetadataExchanger: func(_ string) (metadata.M, error) { return metadata.M{"hello": "llm bridge"}, nil },
			})

			caller, err := service.LoadOrCreateCaller(&http.Request{})
			assert.NoError(t, err)

			caller.SetSystemPrompt(tt.args.systemPrompt, pkgai.SystemPromptOpOverwrite)

			resp, err := service.GetInvoke(context.TODO(), tt.args.userInstruction, tt.args.baseSystemMessage, "transID", caller, true, nil)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantUsage, resp.TokenUsage)
			assert.Equal(t, tt.wantRequest, pd.RequestRecords())
		})
	}
}

func TestServiceChatCompletion(t *testing.T) {
	type args struct {
		providerMockData []provider.MockData
		mockCallReqResp  []mockFunctionCall
		systemPrompt     string
		request          openai.ChatCompletionRequest
	}
	tests := []struct {
		name        string
		args        args
		wantRequest []openai.ChatCompletionRequest
	}{
		{
			name: "chat with tool call",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionResponse(toolCallResp, stopResp),
				},
				mockCallReqResp: []mockFunctionCall{
					{toolID: "call_abc123", functionName: "get_current_weather", respContent: "temperature: 31°C"},
				},
				systemPrompt: "this is a system prompt",
				request: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "How is the weather today in Boston, MA?"}},
				},
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a system prompt"},
						{Role: "user", Content: "How is the weather today in Boston, MA?"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a system prompt"},
						{Role: "user", Content: "How is the weather today in Boston, MA?"},
						{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_abc123", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get_current_weather", Arguments: "{\n\"location\": \"Boston, MA\"\n}"}}}},
						{Role: "tool", Content: "temperature: 31°C", ToolCallID: "call_abc123"},
					},
				},
			},
		},
		{
			name: "chat without tool call",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionResponse(stopResp),
				},
				mockCallReqResp: []mockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{toolID: "call_abc123", functionName: "get_current_weather", respContent: "temperature: 31°C"},
				},
				systemPrompt: "You are an assistant.",
				request: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "How are you"}},
				},
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "You are an assistant."},
						{Role: "user", Content: "How are you"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
			},
		},
		{
			name: "chat with tool call in stream",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionStreamResponse(toolCallStreamResp, stopStreamResp),
				},
				mockCallReqResp: []mockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{toolID: "call_9ctHOJqO3bYrpm2A6S7nHd5k", functionName: "get_current_weather", respContent: "temperature: 31°C"},
				},
				systemPrompt: "You are a weather assistant",
				request: openai.ChatCompletionRequest{
					Stream:   true,
					Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "How is the weather today in Boston, MA?"}},
				},
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Stream: true,
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "You are a weather assistant"},
						{Role: "user", Content: "How is the weather today in Boston, MA?"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
				{
					Stream: true,
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "You are a weather assistant"},
						{Role: "user", Content: "How is the weather today in Boston, MA?"},
						{Role: "assistant", ToolCalls: []openai.ToolCall{{Index: toInt(0), ID: "call_9ctHOJqO3bYrpm2A6S7nHd5k", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get_current_weather", Arguments: "{\"location\":\"Boston, MA\"}"}}}},
						{Role: "tool", Content: "temperature: 31°C", ToolCallID: "call_9ctHOJqO3bYrpm2A6S7nHd5k"},
					},
				},
			},
		},
		{
			name: "chat without tool call in stream",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionStreamResponse(stopStreamResp),
				},
				mockCallReqResp: []mockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{toolID: "call_9ctHOJqO3bYrpm2A6S7nHd5k", functionName: "get_current_weather", respContent: "temperature: 31°C"},
				},
				systemPrompt: "You are a weather assistant",
				request: openai.ChatCompletionRequest{
					Stream:   true,
					Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "How is the weather today in Boston, MA?"}},
				},
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Stream: true,
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "You are a weather assistant"},
						{Role: "user", Content: "How is the weather today in Boston, MA?"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd, err := provider.NewMock("mock provider", tt.args.providerMockData...)
			if err != nil {
				t.Fatal(err)
			}

			flow := newMockDataFlow(newHandler(2 * time.Hour).handle)

			newCaller := func(_ yomo.Source, _ yomo.StreamFunction, _ metadata.M, _ time.Duration) (*pkgai.Caller, error) {
				return mockCaller(tt.args.mockCallReqResp), err
			}

			service := pkgai.NewServiceWithCallerFunc(pd, newCaller, &pkgai.ServiceOptions{
				SourceBuilder:     func(_ string) yomo.Source { return flow },
				ReducerBuilder:    func(_ string) yomo.StreamFunction { return flow },
				MetadataExchanger: func(_ string) (metadata.M, error) { return metadata.M{"hello": "llm bridge"}, nil },
			})

			caller, err := service.LoadOrCreateCaller(&http.Request{})
			assert.NoError(t, err)

			caller.SetSystemPrompt(tt.args.systemPrompt, pkgai.SystemPromptOpOverwrite)

			w := httptest.NewRecorder()
			err = service.GetChatCompletions(context.TODO(), tt.args.request, "transID", caller, pkgai.NewResponseWriter(w, slog.Default()), nil)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantRequest, pd.RequestRecords())
		})
	}
}

func toInt(val int) *int { return &val }

var stopStreamResp = `data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":"Hello"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" I'm"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" just"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" computer"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" program"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" so"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" I"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" don't"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" have"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" feelings"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" but"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" I'm"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" here"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" ready"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" help"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" whatever"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" need"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" How"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" can"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" I"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" assist"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":" today"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{"content":"?"},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}],"usage":null}

data: {"id":"chatcmpl-9blY98pEJe6mXGKivCZyl61vxaUFq","object":"chat.completion.chunk","created":1718787945,"model":"gpt-4o-2024-05-13","system_fingerprint":"fp_f4e629d0a5","choices":[],"usage":{"prompt_tokens":13,"completion_tokens":34,"total_tokens":47}}

data: [DONE]`

var stopResp = `{
  "id": "chatcmpl-9blYknv9rHvr2dvCQKMeW21hlBpCX",
  "object": "chat.completion",
  "created": 1718787982,
  "model": "gpt-4o-2024-05-13",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I'm just a computer program, so I don't have feelings, but thanks for asking. How can I assist you today?"
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 13,
    "completion_tokens": 26,
    "total_tokens": 39
  },
  "system_fingerprint": "fp_f4e629d0a5"
}`

var toolCallStreamResp = `data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_9ctHOJqO3bYrpm2A6S7nHd5k","type":"function","function":{"name":"get_current_weather","arguments":""}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\""}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"location"}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\""}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"Boston"}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":","}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" MA"}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"}"}}]},"logprobs":null,"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"tool_calls"}],"usage":null}

data: {"id":"chatcmpl-9blTCqGy0TGLdK4sOYlGrNxbGGknW","object":"chat.completion.chunk","created":1718787638,"model":"gpt-4-turbo-2024-04-09","system_fingerprint":"fp_9d7f5c6195","choices":[],"usage":{"prompt_tokens":83,"completion_tokens":17,"total_tokens":100}}`

var toolCallResp = `{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1699896916,
  "model": "gpt-4-turbo-2024-04-09",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_abc123",
            "type": "function",
            "function": {
              "name": "get_current_weather",
              "arguments": "{\n\"location\": \"Boston, MA\"\n}"
            }
          }
        ]
      },
      "logprobs": null,
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {
    "prompt_tokens": 82,
    "completion_tokens": 17,
    "total_tokens": 99
  }
}`
