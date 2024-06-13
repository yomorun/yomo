package ai

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/serverless/mock"
)

var testdata = map[uint32][]*openai.ToolCall{
	1: {{ID: "tool-call-id-1", Function: openai.FunctionCall{Name: "function-1"}}},
	2: {{ID: "tool-call-id-2", Function: openai.FunctionCall{Name: "function-2"}}},
	3: {{ID: "tool-call-id-3", Function: openai.FunctionCall{Name: "function-3"}}},
	4: {{ID: "tool-call-id-4", Function: openai.FunctionCall{Name: "function-4"}}},
}

func TestCallSyncer(t *testing.T) {
	wh := newMockWriteHander()
	defer wh.Close()

	syncer := NewCallSyncer(slog.Default(), wh, wh, 0)
	go wh.run()

	var (
		transID = "mock-trans-id"
		reqID   = "mock-req-id"
	)

	time.Sleep(50 * time.Millisecond)
	want := wh.Result()

	got, _ := syncer.Call(context.TODO(), transID, reqID, testdata)
	assert.ElementsMatch(t, want, got)
}

type mockWriteHander struct {
	done    chan struct{}
	wrCh    chan *mock.MockContext
	handler core.AsyncHandler

	mu   sync.Mutex
	ctxs map[*mock.MockContext]struct{}
}

func newMockWriteHander() *mockWriteHander {
	return &mockWriteHander{
		done: make(chan struct{}),
		wrCh: make(chan *mock.MockContext),
		ctxs: make(map[*mock.MockContext]struct{}),
	}
}

func (t *mockWriteHander) Write(tag uint32, data []byte) error {
	t.wrCh <- mock.NewMockContext(data, tag)
	return nil
}

func (t *mockWriteHander) SetHandler(fn core.AsyncHandler) error {
	t.handler = fn
	return nil
}

func (t *mockWriteHander) Close() error { return nil }

func (t *mockWriteHander) run() {
	for c := range t.wrCh {
		t.handler(c)

		t.mu.Lock()
		t.ctxs[c] = struct{}{}
		t.mu.Unlock()
	}
}

func (t *mockWriteHander) Result() []openai.ChatCompletionMessage {
	t.mu.Lock()
	defer t.mu.Unlock()

	want := []openai.ChatCompletionMessage{}
	for c := range t.ctxs {
		invoke, _ := c.LLMFunctionCall()
		want = append(want, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleTool, Content: invoke.Result, ToolCallID: invoke.ToolCallID,
		})
	}

	return want
}
