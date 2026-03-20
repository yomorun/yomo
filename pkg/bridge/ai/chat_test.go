package ai

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/pkg/bridge/ai/caller"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestHandleToolCalls(t *testing.T) {
	type args struct {
		chatCtx   *chatContext
		toolCalls []openai.ToolCall
		reqStream bool
	}
	tests := []struct {
		name         string
		args         args
		wantContinue bool
	}{
		{
			name: "only backend tools",
			args: args{
				chatCtx: &chatContext{
					toolSources: map[string]bool{
						"tool1": true,
					},
				},
				toolCalls: []openai.ToolCall{
					{Function: openai.FunctionCall{Name: "tool1"}},
				},
				reqStream: false,
			},
			wantContinue: true,
		},
		{
			name: "only frontend tools",
			args: args{
				chatCtx: &chatContext{
					toolSources: map[string]bool{
						"toolA": false,
					},
				},
				toolCalls: []openai.ToolCall{
					{Function: openai.FunctionCall{Name: "toolA"}},
				},
				reqStream: false,
			},
			wantContinue: false,
		},
		{
			name: "mixed tools",
			args: args{
				chatCtx: &chatContext{
					toolSources: map[string]bool{
						"tool1": true,  // backend
						"toolA": false, // frontend
					},
				},
				toolCalls: []openai.ToolCall{
					{Function: openai.FunctionCall{Name: "tool1"}},
					{Function: openai.FunctionCall{Name: "toolA"}},
				},
				reqStream: false,
			},
			wantContinue: false,
		},
		{
			name: "unknown tool source",
			args: args{
				chatCtx: &chatContext{
					toolSources: map[string]bool{},
				},
				toolCalls: []openai.ToolCall{
					{Function: openai.FunctionCall{Name: "unknownTool"}},
				},
				reqStream: false,
			},
			wantContinue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock dependencies
			w := httptest.NewRecorder()
			responseWriter := NewResponseWriter(w, slog.Default())

			// Create a mock chat response
			mockResp := &chatResp{
				resp: openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{
						{
							Message: openai.ChatCompletionMessage{
								ToolCalls: tt.args.toolCalls,
							},
						},
					},
				},
			}

			// Create a mock caller
			mockCaller := caller.MockCaller([]caller.MockFunctionCall{
				{FunctionName: "tool1", RespContent: "test response"},
			})

			// Create a noop tracer
			tracer := noop.NewTracerProvider().Tracer("")
			_, span := tracer.Start(context.Background(), "test")

			// Call handleToolCalls
			continueLoop, err := handleToolCalls(
				context.Background(),
				tt.args.chatCtx,
				tt.args.toolCalls,
				tt.args.reqStream,
				responseWriter,
				mockResp,
				span,
				mockCaller,
				tracer,
				"test-trans-id",
				nil,
			)

			// Assert results
			assert.NoError(t, err)
			assert.Equal(t, tt.wantContinue, continueLoop)
		})
	}
}

func TestHandleToolCallsFiltersMixedClientTools(t *testing.T) {
	chatCtx := &chatContext{
		toolSources: map[string]bool{
			"tool1": true,
			"toolA": false,
		},
	}
	toolCalls := []openai.ToolCall{
		{Function: openai.FunctionCall{Name: "tool1"}},
		{Function: openai.FunctionCall{Name: "toolA"}},
	}

	w := httptest.NewRecorder()
	responseWriter := NewResponseWriter(w, slog.Default())

	mockResp := &chatResp{
		resp: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						ToolCalls: toolCalls,
					},
				},
			},
		},
	}

	mockCaller := caller.MockCaller([]caller.MockFunctionCall{
		{FunctionName: "tool1", RespContent: "test response"},
	})

	tracer := noop.NewTracerProvider().Tracer("")
	_, span := tracer.Start(context.Background(), "test")

	continueLoop, err := handleToolCalls(
		context.Background(),
		chatCtx,
		toolCalls,
		false,
		responseWriter,
		mockResp,
		span,
		mockCaller,
		tracer,
		"test-trans-id",
		nil,
	)

	assert.NoError(t, err)
	assert.False(t, continueLoop)

	var got openai.ChatCompletionResponse
	decodeErr := json.Unmarshal(w.Body.Bytes(), &got)
	assert.NoError(t, decodeErr)
	if assert.NotEmpty(t, got.Choices) {
		toolCalls := got.Choices[0].Message.ToolCalls
		if assert.Len(t, toolCalls, 1) {
			assert.Equal(t, "toolA", toolCalls[0].Function.Name)
		}
	}
}

func TestWriteClientToolCallsResponseStreamFiltersClientTools(t *testing.T) {
	resp := &streamChatResp{
		buffer: []openai.ChatCompletionStreamResponse{
			{
				ID: "chatcmpl-1",
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{Role: "assistant"},
					},
				},
			},
			{
				ID: "chatcmpl-1",
				Choices: []openai.ChatCompletionStreamChoice{
					{
						FinishReason: openai.FinishReasonToolCalls,
						Delta:        openai.ChatCompletionStreamChoiceDelta{},
					},
				},
			},
		},
		toolCallDeltas: []openai.ChatCompletionStreamResponse{
			{
				ID: "chatcmpl-1",
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							ToolCalls: []openai.ToolCall{
								{Index: toInt(0), Function: openai.FunctionCall{Name: "tool1"}},
								{Index: toInt(1), Function: openai.FunctionCall{Name: "toolA"}},
							},
						},
					},
				},
			},
		},
	}

	clientToolCalls := []openai.ToolCall{{Index: toInt(1), Function: openai.FunctionCall{Name: "toolA"}}}

	w := httptest.NewRecorder()
	responseWriter := NewResponseWriter(w, slog.Default())
	responseWriter.SetStreamHeader()

	err := writeClientToolCallsResponse(responseWriter, &chatContext{}, resp, clientToolCalls)
	assert.NoError(t, err)

	got := w.Body.String()
	assert.True(t, strings.Contains(got, "toolA"))
	assert.False(t, strings.Contains(got, "tool1"))
	assert.True(t, strings.Contains(got, "[DONE]"))
}

type mockStreamRecver struct {
	items []openai.ChatCompletionStreamResponse
}

func (m *mockStreamRecver) Recv() (openai.ChatCompletionStreamResponse, error) {
	if len(m.items) == 0 {
		return openai.ChatCompletionStreamResponse{}, io.EOF
	}
	item := m.items[0]
	m.items = m.items[1:]
	return item, nil
}

func TestStreamToolCallsImmediateForClientTools(t *testing.T) {
	recver := &mockStreamRecver{items: []openai.ChatCompletionStreamResponse{
		{
			ID: "chatcmpl-1",
			Choices: []openai.ChatCompletionStreamChoice{
				{Delta: openai.ChatCompletionStreamChoiceDelta{Role: "assistant"}},
			},
		},
		{
			ID: "chatcmpl-1",
			Choices: []openai.ChatCompletionStreamChoice{
				{Delta: openai.ChatCompletionStreamChoiceDelta{ToolCalls: []openai.ToolCall{{Index: toInt(0), Function: openai.FunctionCall{Name: "toolA", Arguments: "{}"}}}}},
			},
		},
		{
			ID: "chatcmpl-1",
			Choices: []openai.ChatCompletionStreamChoice{
				{FinishReason: openai.FinishReasonToolCalls, Delta: openai.ChatCompletionStreamChoiceDelta{}},
			},
		},
	}}

	resp := &streamChatResp{
		recver:       recver,
		toolCallsMap: make(map[int]openai.ToolCall),
	}

	chatCtx := &chatContext{
		callTimes: 1,
		toolSources: map[string]bool{
			"toolA": false,
		},
	}

	w := httptest.NewRecorder()
	responseWriter := NewResponseWriter(w, slog.Default())

	result, err := resp.process(responseWriter, chatCtx)
	assert.NoError(t, err)
	assert.True(t, result.isFunctionCall)

	got := w.Body.String()
	idxTool := strings.Index(got, "toolA")
	idxDone := strings.Index(got, "[DONE]")
	assert.True(t, idxTool >= 0)
	assert.True(t, idxDone > idxTool)
}
