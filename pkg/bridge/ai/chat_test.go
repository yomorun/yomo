package ai

import (
	"context"
	"log/slog"
	"net/http/httptest"
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
