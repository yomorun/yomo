package caller

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"go.opentelemetry.io/otel/trace"
)

// MockCaller returns a mock
// the request-response of caller has been defined in advance, the request and response are defined in the `calls`.
func MockCaller(calls []MockFunctionCall) *Caller {
	// register function to register
	for connID, call := range calls {
		ai.RegisterFunction(&openai.FunctionDefinition{Name: call.FunctionName}, uint64(connID), nil)
	}

	caller := &Caller{
		CallSyncer: &MockCallSyncer{calls: calls},
	}

	return caller
}

// MockFunctionCall holds the result of a mock function call. This definition helps test cases easily access tool call results.
type MockFunctionCall struct {
	ToolID       string
	FunctionName string
	RespContent  string
}

type MockCallSyncer struct {
	calls []MockFunctionCall
}

// Call implements CallSyncer, it returns the mock response defined in advance.
func (m *MockCallSyncer) Call(ctx context.Context, transID string, reqID string, _ []byte, toolCalls []openai.ToolCall, _ trace.Tracer) ([]ai.ToolCallResult, error) {
	res := []ai.ToolCallResult{}

	for _, call := range m.calls {
		res = append(res, ai.ToolCallResult{
			FunctionName: call.FunctionName,
			ToolCallID:   call.ToolID,
			Content:      call.RespContent,
		})
	}
	return res, nil
}

func (m *MockCallSyncer) Close() error { return nil }
