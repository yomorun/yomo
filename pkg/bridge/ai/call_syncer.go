package ai

import (
	"context"
	"log/slog"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/serverless"
)

// CallSyncer fire a bunch of function callings, and wait the result of these function callings.
// every tool call has a ID, which is used to identify the function calling, if Caller fired it,
// The WaitResult will wait the result that all of the function callings responded.
// Note: one tool call can only be responded once.
type CallSyncer interface {
	// Call(transID, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) ([]openai.ChatCompletionMessage, error)

	// Fire fire a bunch of function callings, it will return immediately,
	// you should call WaitResult to wait the result of these function callings.
	Fire(transID, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) error

	// WaitResult will wait the result all of the function callings responded.
	// The result only contains the messages with role=="tool".
	WaitResult(ctx context.Context, transID, reqID string) ([]openai.ChatCompletionMessage, error)

	// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
	Close() error
}

// TagWriter write tag and []byte.
type (
	TagWriter interface {
		Write(tag uint32, data []byte) error
		Close() error
	}

	// Reducer handle the tag and []byte.
	Reducer interface {
		SetHandler(fn core.AsyncHandler) error
		Close() error
	}
)

type callSyncer struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	// timeout is the timeout for waiting the result.
	timeout   time.Duration
	writer    TagWriter
	reducer   Reducer
	receiveCh <-chan ReqMessage

	// internal use
	reqToolsCh chan reqTools
	reqMsgChCh chan reqMsgCh
}

type ReqMessage struct {
	ReqID   string
	Message openai.ChatCompletionMessage
}

func NewCallSyncer(logger *slog.Logger, writer TagWriter, reducer Reducer, timeout time.Duration) CallSyncer {
	if timeout == 0 {
		timeout = RunFunctionTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())

	syncer := &callSyncer{
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
		timeout:    timeout,
		writer:     writer,
		reducer:    reducer,
		receiveCh:  handleToChan(logger, reducer),
		reqToolsCh: make(chan reqTools),
		reqMsgChCh: make(chan reqMsgCh),
	}

	go syncer.background()

	return syncer
}

type (
	reqTools struct {
		reqID   string
		toolIDs map[string]struct{}
	}

	reqMsgCh struct {
		reqID string
		ch    chan []openai.ChatCompletionMessage
	}
)

func (f *callSyncer) Fire(transID string, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) error {
	toolIDs := make(map[string]struct{})

	for tag, tools := range tagToolCalls {
		f.logger.Debug("invoke toolCalls", "tag", tag, "len(tools)", len(tools), "transID", transID, "reqID", reqID)

		for _, t := range tools {
			data := &ai.FunctionCall{
				TransID:      transID,
				ReqID:        reqID,
				ToolCallID:   t.ID,
				FunctionName: t.Function.Name,
				Arguments:    t.Function.Arguments,
			}
			buf, _ := data.Bytes()

			if err := f.writer.Write(tag, buf); err != nil {
				f.logger.Error("send data to zipper", "err", err.Error())
				continue
			}
			toolIDs[t.ID] = struct{}{}
		}
	}

	f.reqToolsCh <- reqTools{reqID: reqID, toolIDs: toolIDs}

	return nil
}

// WaitResult implements FunctionCaller.
func (f *callSyncer) WaitResult(ctx context.Context, transID string, reqID string) ([]openai.ChatCompletionMessage, error) {
	ch := make(chan []openai.ChatCompletionMessage)

	reqMsg := reqMsgCh{
		reqID: reqID,
		ch:    ch,
	}

	f.reqMsgChCh <- reqMsg

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

// Close implements CallSyncer.
func (f *callSyncer) Close() error {
	f.cancel()

	var err error
	if err = f.writer.Close(); err != nil {
		f.logger.Error("callSyncer writer close", "err", err.Error())
	}

	if err = f.reducer.Close(); err != nil {
		f.logger.Error("callSyncer reducer close", "err", err.Error())
	}

	return err
}

type item struct {
	deadline time.Time
	// key is toolCallID
	messages map[string]*openai.ChatCompletionMessage
}

func (f *callSyncer) dispatch(
	reqID string,
	reqs map[string]*item,
	msgs map[string]map[string]openai.ChatCompletionMessage,
	resch map[string]chan []openai.ChatCompletionMessage,
) {
	// not fired.
	tool, ok := reqs[reqID]
	if !ok {
		return
	}

	// deadline expired
	if tool.deadline.Before(time.Now()) {
		for toolID, msg := range tool.messages {
			if msg != nil {
				continue
			}
			tool.messages[toolID] = &openai.ChatCompletionMessage{
				ToolCallID: toolID,
				Role:       openai.ChatMessageRoleTool,
				Content:    "timeout in this function calling, you should ignore this. ",
			}
		}
	}

	var result []openai.ChatCompletionMessage
	i := 0
	for _, msg := range tool.messages {
		if msg == nil {
			f.logger.Debug("dispatch", "reqID", reqID, "fired", len(tool.messages), "received", i)
			return
		}
		result = append(result, *msg)
		i++
	}

	ch, ok := resch[reqID]
	if !ok {
		return
	}

	select {
	case ch <- result:
		delete(reqs, reqID)
		delete(msgs, reqID)
		delete(resch, reqID)
		f.logger.Debug("dispatch", "reqID", reqID, "fired", len(tool.messages), "received", i)
	default:
	}
}

func (f *callSyncer) background() {
	// reqs stores fire request, the key is the reqID
	reqs := make(map[string]*item)
	// msgs stores the messages from the reducer, the key is the reqID
	msgs := make(map[string]map[string]openai.ChatCompletionMessage)
	// resChs stores the result channel, the key is the reqID, the value channel will be fulled when the result comes.
	resChs := make(map[string]chan []openai.ChatCompletionMessage)

	for {
		select {
		case <-f.ctx.Done():
			return
		case reqTools := <-f.reqToolsCh:
			item := &item{
				deadline: time.Now().Add(f.timeout),
				messages: make(map[string]*openai.ChatCompletionMessage),
			}
			for toolID := range reqTools.toolIDs {
				item.messages[toolID] = nil
			}

			for k, v := range msgs[reqTools.reqID] {
				item.messages[k] = &openai.ChatCompletionMessage{
					ToolCallID: v.ToolCallID,
					Role:       v.Role,
					Content:    v.Content,
				}
			}
			reqs[reqTools.reqID] = item

			f.dispatch(reqTools.reqID, reqs, msgs, resChs)

		case rc := <-f.reqMsgChCh:
			resChs[rc.reqID] = rc.ch

			f.dispatch(rc.reqID, reqs, msgs, resChs)

		case msg := <-f.receiveCh:
			tool, ok := reqs[msg.ReqID]
			if !ok {
				_, ok := msgs[msg.ReqID]
				if !ok {
					msgs[msg.ReqID] = make(map[string]openai.ChatCompletionMessage)
				}
				msgs[msg.ReqID][msg.Message.ToolCallID] = openai.ChatCompletionMessage{
					ToolCallID: msg.Message.ToolCallID,
					Role:       msg.Message.Role,
					Content:    msg.Message.Content,
				}
				continue
			}
			tool.messages[msg.Message.ToolCallID] = &msg.Message

			f.dispatch(msg.ReqID, reqs, msgs, resChs)
		}
	}
}

func handleToChan(logger *slog.Logger, reducer Reducer) <-chan ReqMessage {
	ch := make(chan ReqMessage)

	reducer.SetHandler(func(ctx serverless.Context) {
		buf := ctx.Data()

		logger.Debug("[sfn-reducer]", "tag", ai.ReducerTag, "data", string(buf))
		invoke, err := ctx.LLMFunctionCall()
		if err != nil {
			logger.Error("[sfn-reducer] parse function calling invoke", "err", err.Error())
			return
		}

		message := openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    invoke.Result,
			ToolCallID: invoke.ToolCallID,
		}

		logger.Debug("[sfn-reducer] generate", "tool_call_id", message.ToolCallID, "content", message.Content)

		ch <- ReqMessage{ReqID: invoke.ReqID, Message: message}
	})

	return ch
}
