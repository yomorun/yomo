package mock

import (
	"sync"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/mock"
)

// Handler.handle implements core.AsyncHandler, it just echo the context be written.
type Handler struct {
	sleep time.Duration
	mu    sync.Mutex
	ctxs  map[*mock.MockContext]struct{}
}

func NewHandler(sleep time.Duration) *Handler {
	return &Handler{
		sleep: sleep,
		ctxs:  make(map[*mock.MockContext]struct{}),
	}
}

func (h *Handler) Handle(c serverless.Context) {
	time.Sleep(h.sleep)

	h.mu.Lock()
	defer h.mu.Unlock()
	h.ctxs[c.(*mock.MockContext)] = struct{}{}
}

func (h *Handler) Result() []ai.ToolCallResult {
	h.mu.Lock()
	defer h.mu.Unlock()

	want := []ai.ToolCallResult{}
	for c := range h.ctxs {
		invoke, _ := c.LLMFunctionCall()
		want = append(want, ai.ToolCallResult{
			FunctionName: invoke.FunctionName, Content: invoke.Result, ToolCallID: invoke.ToolCallID,
		})
	}

	return want
}

// DataFlow mocks the data flow of llm bridge.
// The data flow is: source -> hander -> reducer,
// It is `Write() -> handler() -> reducer()` in this mock implementation.
type DataFlow struct {
	wrCh    chan *mock.MockContext
	reducer core.AsyncHandler
	handler core.AsyncHandler
}

func NewDataFlow(handler core.AsyncHandler) *DataFlow {
	return &DataFlow{
		wrCh:    make(chan *mock.MockContext),
		handler: handler,
	}
}

func (t *DataFlow) Write(tag uint32, data []byte) error {
	t.wrCh <- mock.NewMockContext(data, tag)
	return nil
}

func (t *DataFlow) WriteWithTarget(tag uint32, data []byte, target string) error {
	t.wrCh <- mock.NewMockContext(data, tag)
	return nil
}

func (t *DataFlow) SetHandler(fn core.AsyncHandler) error {
	t.reducer = fn
	return nil
}

func (t *DataFlow) Close() error { return nil }

// this function explains how the data flow works,
// it receives data from the write channel, and handle with the handler, then send the result to the reducer.
func (t *DataFlow) Run() {
	for c := range t.wrCh {
		t.handler(c)
		t.reducer(c)
	}
}

var (
	_ yomo.Source         = (*DataFlow)(nil)
	_ yomo.StreamFunction = (*DataFlow)(nil)
)

// The test will not use blowing function in this mock implementation.
func (t *DataFlow) SetObserveDataTags(tag ...uint32)                      {}
func (t *DataFlow) Connect() error                                        { return nil }
func (t *DataFlow) Init(fn func() error) error                            { panic("unimplemented") }
func (t *DataFlow) SetCronHandler(spec string, fn core.CronHandler) error { panic("unimplemented") }
func (t *DataFlow) SetPipeHandler(fn core.PipeHandler) error              { panic("unimplemented") }
func (t *DataFlow) SetWantedTarget(string)                                { panic("unimplemented") }
func (t *DataFlow) Wait()                                                 { panic("unimplemented") }
func (t *DataFlow) SetErrorHandler(fn func(err error))                    { panic("unimplemented") }
