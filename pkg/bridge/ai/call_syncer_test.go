package ai

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/mock"
)

var testdata = map[uint32][]*openai.ToolCall{
	1: {{ID: "tool-call-id-1", Function: openai.FunctionCall{Name: "function-1"}}},
	2: {{ID: "tool-call-id-2", Function: openai.FunctionCall{Name: "function-2"}}},
	3: {{ID: "tool-call-id-3", Function: openai.FunctionCall{Name: "function-3"}}},
	4: {{ID: "tool-call-id-4", Function: openai.FunctionCall{Name: "function-4"}}},
}

func TestTimeoutCallSyncer(t *testing.T) {
	h := newHandler(2 * time.Hour) // h.sleep > syncer.timeout
	flow := newMockDataFlow(h.handle)
	defer flow.Close()

	req, _ := sourceWriteToChan(flow, slog.Default())
	res, _ := reduceToChan(flow, slog.Default())

	syncer := NewCallSyncer(slog.Default(), req, res, time.Millisecond)
	go flow.run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	want := []ToolCallResult{
		{
			FunctionName: "timeout-function",
			ToolCallID:   "tool-call-id",
			Content:      "timeout in this function calling, you should ignore this.",
		},
	}

	got, _ := syncer.Call(context.TODO(), transID, reqID, map[uint32][]*openai.ToolCall{
		1: {{ID: "tool-call-id", Function: openai.FunctionCall{Name: "timeout-function"}}},
	})

	assert.ElementsMatch(t, want, got)
}

func TestCallSyncer(t *testing.T) {
	h := newHandler(0)
	flow := newMockDataFlow(h.handle)
	defer flow.Close()

	req, _ := sourceWriteToChan(flow, slog.Default())
	res, _ := reduceToChan(flow, slog.Default())

	syncer := NewCallSyncer(slog.Default(), req, res, 0)
	go flow.run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	got, _ := syncer.Call(context.TODO(), transID, reqID, testdata)

	assert.NotEmpty(t, got)
	assert.ElementsMatch(t, h.result(), got)
}

// handler.handle implements core.AsyncHandler, it just echo the context be written.
type handler struct {
	sleep time.Duration
	mu    sync.Mutex
	ctxs  map[*mock.MockContext]struct{}
}

func newHandler(sleep time.Duration) *handler {
	return &handler{
		sleep: sleep,
		ctxs:  make(map[*mock.MockContext]struct{}),
	}
}

func (h *handler) handle(c serverless.Context) {
	time.Sleep(h.sleep)

	h.mu.Lock()
	defer h.mu.Unlock()
	h.ctxs[c.(*mock.MockContext)] = struct{}{}
}

func (h *handler) result() []ToolCallResult {
	h.mu.Lock()
	defer h.mu.Unlock()

	want := []ToolCallResult{}
	for c := range h.ctxs {
		invoke, _ := c.LLMFunctionCall()
		want = append(want, ToolCallResult{
			FunctionName: invoke.FunctionName, Content: invoke.Result, ToolCallID: invoke.ToolCallID,
		})
	}

	return want
}

// mockDataFlow mocks the data flow of llm bridge.
// The data flow is: source -> hander -> reducer,
// It is `Write() -> handler() -> reducer()` in this mock implementation.
type mockDataFlow struct {
	wrCh    chan *mock.MockContext
	reducer core.AsyncHandler
	handler core.AsyncHandler
}

func newMockDataFlow(handler core.AsyncHandler) *mockDataFlow {
	return &mockDataFlow{
		wrCh:    make(chan *mock.MockContext),
		handler: handler,
	}
}

func (t *mockDataFlow) Write(tag uint32, data []byte) error {
	t.wrCh <- mock.NewMockContext(data, tag)
	return nil
}

func (t *mockDataFlow) SetHandler(fn core.AsyncHandler) error {
	t.reducer = fn
	return nil
}

func (t *mockDataFlow) Close() error { return nil }

// this function explains how the data flow works,
// it receives data from the write channel, and handle with the handler, then send the result to the reducer.
func (t *mockDataFlow) run() {
	for c := range t.wrCh {
		t.handler(c)
		t.reducer(c)
	}
}

var _ yomo.Source = (*mockDataFlow)(nil)
var _ yomo.StreamFunction = (*mockDataFlow)(nil)

// The test will not use blowing function in this mock implementation.
func (t *mockDataFlow) SetObserveDataTags(tag ...uint32)                      {}
func (t *mockDataFlow) Connect() error                                        { return nil }
func (t *mockDataFlow) Init(fn func() error) error                            { panic("unimplemented") }
func (t *mockDataFlow) SetCronHandler(spec string, fn core.CronHandler) error { panic("unimplemented") }
func (t *mockDataFlow) SetPipeHandler(fn core.PipeHandler) error              { panic("unimplemented") }
func (t *mockDataFlow) SetWantedTarget(string)                                { panic("unimplemented") }
func (t *mockDataFlow) Wait()                                                 { panic("unimplemented") }
func (t *mockDataFlow) SetErrorHandler(fn func(err error))                    { panic("unimplemented") }
func (t *mockDataFlow) WriteWithTarget(_ uint32, _ []byte, _ string) error    { panic("unimplemented") }
