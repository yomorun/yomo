package caller

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/mock"
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
	h := mock.NewHandler(2 * time.Hour) // h.sleep > syncer.timeout
	flow := mock.NewDataFlow(h.Handle)
	defer flow.Close()

	req, _ := SourceWriteToChan(flow, slog.Default())
	res, _ := ReduceToChan(flow, slog.Default())

	syncer := NewCallSyncer(slog.Default(), req, res, time.Millisecond)
	go flow.Run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	want := []ai.ToolCallResult{
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
	h := mock.NewHandler(0)
	flow := mock.NewDataFlow(h.Handle)
	defer flow.Close()

	req, _ := SourceWriteToChan(flow, slog.Default())
	res, _ := ReduceToChan(flow, slog.Default())

	syncer := NewCallSyncer(slog.Default(), req, res, 0)
	go flow.Run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	got, _ := syncer.Call(context.TODO(), transID, reqID, nil, testdata, noopTracer)

	assert.NotEmpty(t, got)
	assert.ElementsMatch(t, h.Result(), got)
}
