package test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
	"go.opentelemetry.io/otel/trace/noop"
)

var testdata = []openai.ToolCall{
	{ID: "tool-call-id-1", Function: openai.FunctionCall{Name: "function-1"}},
	{ID: "tool-call-id-2", Function: openai.FunctionCall{Name: "function-2"}},
	{ID: "tool-call-id-3", Function: openai.FunctionCall{Name: "function-3"}},
	{ID: "tool-call-id-4", Function: openai.FunctionCall{Name: "function-4"}},
}

var noopTracer = noop.NewTracerProvider().Tracer("for_test")

func TestTimeoutCallSyncer(t *testing.T) {
	h := newHandler(2 * time.Hour) // h.sleep > syncer.timeout
	flow := newMockDataFlow(h.handle)
	defer flow.Close()

	req, _ := pkgai.SourceWriteToChan(flow, slog.Default())
	res, _ := pkgai.ReduceToChan(flow, slog.Default())

	syncer := pkgai.NewCallSyncer(slog.Default(), req, res, time.Millisecond)
	go flow.Run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	want := []pkgai.ToolCallResult{
		{
			FunctionName: "timeout-function",
			ToolCallID:   "tool-call-id",
			Content:      "timeout in this function calling, you should ignore this.",
		},
	}

	got, _ := syncer.Call(context.TODO(), transID, reqID,
		nil,
		[]openai.ToolCall{
			{ID: "tool-call-id", Function: openai.FunctionCall{Name: "timeout-function"}},
		},
		noopTracer)

	assert.ElementsMatch(t, want, got)
}

func TestCallSyncer(t *testing.T) {
	h := newHandler(0)
	flow := newMockDataFlow(h.handle)
	defer flow.Close()

	req, _ := pkgai.SourceWriteToChan(flow, slog.Default())
	res, _ := pkgai.ReduceToChan(flow, slog.Default())

	syncer := pkgai.NewCallSyncer(slog.Default(), req, res, 0)
	go flow.Run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	got, _ := syncer.Call(context.TODO(), transID, reqID, nil, testdata, noopTracer)

	assert.NotEmpty(t, got)
	assert.ElementsMatch(t, h.Result(), got)
}
