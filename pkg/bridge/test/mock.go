package test

import (
	"context"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/mock"
)

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

func (h *handler) Result() []pkgai.ToolCallResult {
	h.mu.Lock()
	defer h.mu.Unlock()

	want := []pkgai.ToolCallResult{}
	for c := range h.ctxs {
		invoke, _ := c.LLMFunctionCall()
		want = append(want, pkgai.ToolCallResult{
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

func (t *mockDataFlow) WriteWithTarget(tag uint32, data []byte, target string) error {
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
func (t *mockDataFlow) Run() {
	for c := range t.wrCh {
		t.handler(c)
		t.reducer(c)
	}
}

var (
	_ yomo.Source         = (*mockDataFlow)(nil)
	_ yomo.StreamFunction = (*mockDataFlow)(nil)
)

// The test will not use blowing function in this mock implementation.
func (t *mockDataFlow) SetObserveDataTags(tag ...uint32)                      {}
func (t *mockDataFlow) Connect() error                                        { return nil }
func (t *mockDataFlow) Init(fn func() error) error                            { panic("unimplemented") }
func (t *mockDataFlow) SetCronHandler(spec string, fn core.CronHandler) error { panic("unimplemented") }
func (t *mockDataFlow) SetPipeHandler(fn core.PipeHandler) error              { panic("unimplemented") }
func (t *mockDataFlow) SetWantedTarget(string)                                { panic("unimplemented") }
func (t *mockDataFlow) Wait()                                                 { panic("unimplemented") }
func (t *mockDataFlow) SetErrorHandler(fn func(err error))                    { panic("unimplemented") }

// mockCaller returns a mock caller.
// the request-response of caller has been defined in advance, the request and response are defined in the `calls`.
func mockCaller(calls []mockFunctionCall) *pkgai.Caller {
	// register function to register
	for connID, call := range calls {
		ai.RegisterFunction(&openai.FunctionDefinition{Name: call.functionName}, uint64(connID), nil)
	}

	// caller, _ := pkgai.NewCaller(nil, nil, metadata.M{"hello": "llm bridge"}, pkgai.RunFunctionTimeout)
	// callSyncer := &mockCallSyncer{calls: calls}
	// caller.CallSyncer = callSyncer
	caller := &pkgai.Caller{
		CallSyncer: &mockCallSyncer{calls: calls},
		// md:         metadata.M{"hello": "llm bridge"},
	}

	return caller
}

type mockFunctionCall struct {
	toolID       string
	functionName string
	respContent  string
}

type mockCallSyncer struct {
	calls []mockFunctionCall
}

// Call implements CallSyncer, it returns the mock response defined in advance.
func (m *mockCallSyncer) Call(ctx context.Context, transID string, reqID string, toolCalls []openai.ToolCall) ([]pkgai.ToolCallResult, error) {
	res := []pkgai.ToolCallResult{}

	for _, call := range m.calls {
		res = append(res, pkgai.ToolCallResult{
			FunctionName: call.functionName,
			ToolCallID:   call.toolID,
			Content:      call.respContent,
		})
	}
	return res, nil
}

func (m *mockCallSyncer) Close() error { return nil }
