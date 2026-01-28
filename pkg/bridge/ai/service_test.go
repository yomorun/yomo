package ai

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/caller"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/pkg/bridge/mock"
)

func TestOpSystemPrompt(t *testing.T) {
	type args struct {
		prompt string
		op     caller.SystemPromptOp
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
				op:     caller.SystemPromptOpDisabled,
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
				op:     caller.SystemPromptOpOverwrite,
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
				op:     caller.SystemPromptOpOverwrite,
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
				op:     caller.SystemPromptOpOverwrite,
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
				op:     caller.SystemPromptOpPrefix,
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
				op:     caller.SystemPromptOpPrefix,
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
		{
			name: "client preferred with client system prompt",
			args: args{
				prompt: "system prompt",
				op:     caller.SystemPromptOpClientPreferred,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "client prompt"},
						{Role: "user", Content: "test"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "system", Content: "client prompt"},
					{Role: "user", Content: "test"},
				},
			},
		},
		{
			name: "client preferred without client system prompt",
			args: args{
				prompt: "system prompt",
				op:     caller.SystemPromptOpClientPreferred,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "system", Content: "system prompt"},
					{Role: "user", Content: "test"},
				},
			},
		},
		{
			name: "client preferred with empty system prompt",
			args: args{
				prompt: "",
				op:     caller.SystemPromptOpClientPreferred,
				req: openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test"},
					},
				},
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "test"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ServiceOptions{Logger: slog.Default()}
			s := NewService(nil, opts)
			got := s.OpSystemPrompt(tt.args.req, tt.args.prompt, tt.args.op)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServiceInvoke(t *testing.T) {
	type args struct {
		providerMockData []provider.MockData
		mockCallReqResp  []caller.MockFunctionCall
		systemPrompt     string
		userInstruction  string
	}
	tests := []struct {
		name        string
		args        args
		wantRequest []openai.ChatCompletionRequest
		wantUsage   openai.Usage
	}{
		{
			name: "invoke with tool call",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionResponse(mock.ToolCallResp, mock.StopResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{ToolID: "call_abc123", FunctionName: "get_current_weather", RespContent: "temperature: 31°C"},
				},
				systemPrompt:    "this is a system prompt",
				userInstruction: "hi",
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a system prompt"},
						{Role: "user", Content: "hi"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a system prompt"},
						{Role: "user", Content: "hi"},
						{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_abc123", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get_current_weather", Arguments: "{\n\"location\": \"Boston, MA\"\n}"}}}},
						{Role: "tool", Content: "temperature: 31°C", ToolCallID: "call_abc123"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
			},
			wantUsage: openai.Usage{PromptTokens: 95, CompletionTokens: 43, TotalTokens: 138},
		},
		{
			name: "invoke without tool call",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionResponse(mock.StopResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{},
				systemPrompt:    "this is a system prompt",
				userInstruction: "hi",
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "this is a system prompt"},
						{Role: "user", Content: "hi"},
					},
				},
			},
			wantUsage: openai.Usage{PromptTokens: 13, CompletionTokens: 26, TotalTokens: 39},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ai.SetRegister(register.NewDefault(nil))

			pd, err := provider.NewMock("mock provider", tt.args.providerMockData...)
			if err != nil {
				t.Fatal(err)
			}

			flow := mock.NewDataFlow(mock.NewHandler(2 * time.Hour).Handle)

			newCaller := func(_ yomo.Source, _ yomo.StreamFunction, _ metadata.M, _ time.Duration) (*caller.Caller, error) {
				return caller.MockCaller(tt.args.mockCallReqResp), nil
			}

			service := NewServiceWithCallerFunc(pd, newCaller, &ServiceOptions{
				SourceBuilder:     func(_ string) yomo.Source { return flow },
				ReducerBuilder:    func(_ string) yomo.StreamFunction { return flow },
				MetadataExchanger: func(_ string) (metadata.M, error) { return metadata.M{"hello": "llm bridge"}, nil },
			})

			c, err := service.LoadOrCreateCaller(&http.Request{})
			assert.NoError(t, err)

			c.SetSystemPrompt(tt.args.systemPrompt, caller.SystemPromptOpOverwrite)

			w := httptest.NewRecorder()
			err = service.GetInvoke(context.TODO(), tt.args.userInstruction, "transID", c, true, nil, NewResponseWriter(w, slog.Default()), nil)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantRequest, pd.RequestRecords())
		})
	}
}

func TestServiceChatCompletion(t *testing.T) {
	type args struct {
		providerMockData []provider.MockData
		mockCallReqResp  []caller.MockFunctionCall
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
					provider.MockChatCompletionResponse(mock.ToolCallResp, mock.StopResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{ToolID: "call_abc123", FunctionName: "get_current_weather", RespContent: "temperature: 31°C"},
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
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
			},
		},
		{
			name: "chat without tool call",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionResponse(mock.StopResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{ToolID: "call_abc123", FunctionName: "get_current_weather", RespContent: "temperature: 31°C"},
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
					provider.MockChatCompletionStreamResponse(mock.ToolCallStreamResp, mock.StopStreamResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{ToolID: "call_9ctHOJqO3bYrpm2A6S7nHd5k", FunctionName: "get_current_weather", RespContent: "temperature: 31°C"},
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
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "get_current_weather"}}},
				},
			},
		},
		{
			name: "chat without tool call in stream",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionStreamResponse(mock.StopStreamResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{ToolID: "call_9ctHOJqO3bYrpm2A6S7nHd5k", FunctionName: "get_current_weather", RespContent: "temperature: 31°C"},
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
		{
			name: "deepseek-v3.2 stream with tools",
			args: args{
				providerMockData: []provider.MockData{
					provider.MockChatCompletionStreamResponse(mock.Deepseekv32ToolsResp, mock.StopStreamResp),
				},
				mockCallReqResp: []caller.MockFunctionCall{
					// toolID should equal to toolCallResp's toolID
					{ToolID: "call_a6dea19b4490485bbdad7047", FunctionName: "market-get-weather", RespContent: "temperature: 31°C"},
				},
				systemPrompt: "You are a weather assistant",
				request: openai.ChatCompletionRequest{
					Stream:   true,
					Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "how is the weather in tokyo?"}},
				},
			},
			wantRequest: []openai.ChatCompletionRequest{
				{
					Stream: true,
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "You are a weather assistant"},
						{Role: "user", Content: "how is the weather in tokyo?"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "market-get-weather"}}},
				},
				{
					Stream: true,
					Messages: []openai.ChatCompletionMessage{
						{Role: "system", Content: "You are a weather assistant"},
						{Role: "user", Content: "how is the weather in tokyo?"},
						{Role: "assistant", ToolCalls: []openai.ToolCall{{Index: toInt(0), ID: "call_a6dea19b4490485bbdad7047", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "market-get-weather", Arguments: "{\"city\": \"Tokyo\", \"latitude\": 35.6762, \"longitude\": 139.6503}"}}}},
						{Role: "tool", Content: "temperature: 31°C", ToolCallID: "call_a6dea19b4490485bbdad7047"},
					},
					Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "market-get-weather"}}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ai.SetRegister(register.NewDefault(nil))

			pd, err := provider.NewMock("mock provider", tt.args.providerMockData...)
			if err != nil {
				t.Fatal(err)
			}

			flow := mock.NewDataFlow(mock.NewHandler(2 * time.Hour).Handle)

			newCaller := func(_ yomo.Source, _ yomo.StreamFunction, _ metadata.M, _ time.Duration) (*caller.Caller, error) {
				return caller.MockCaller(tt.args.mockCallReqResp), nil
			}

			service := NewServiceWithCallerFunc(pd, newCaller, &ServiceOptions{
				SourceBuilder:     func(_ string) yomo.Source { return flow },
				ReducerBuilder:    func(_ string) yomo.StreamFunction { return flow },
				MetadataExchanger: func(_ string) (metadata.M, error) { return metadata.M{"hello": "llm bridge"}, nil },
			})

			c, err := service.LoadOrCreateCaller(&http.Request{})
			assert.NoError(t, err)

			c.SetSystemPrompt(tt.args.systemPrompt, caller.SystemPromptOpOverwrite)

			w := httptest.NewRecorder()
			err = service.GetChatCompletions(context.TODO(), tt.args.request, "transID", nil, c, NewResponseWriter(w, slog.Default()), nil)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantRequest, pd.RequestRecords())
		})
	}
}

func toInt(val int) *int { return &val }
