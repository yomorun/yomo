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

// CallSyncer fires a bunch of function callings, and wait the result of these function callings.
// every tool call has a toolCallID, which is used to identify the function calling,
// Note that one tool call can only be responded once.
type CallSyncer struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	// timeout is the timeout for waiting the result.
	timeout  time.Duration
	writer   TagWriter
	reducer  Reducer
	reduceCh <-chan reqMessage

	// internal use
	reqToolsCh chan reqTools
	reqMsgChCh chan reqMsgCh
}

type reqMessage struct {
	reqID   string
	message openai.ChatCompletionMessage
}

// NewCallSyncer creates a new CallSyncer.
func NewCallSyncer(logger *slog.Logger, writer TagWriter, reducer Reducer, timeout time.Duration) *CallSyncer {
	if timeout == 0 {
		timeout = RunFunctionTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())

	syncer := &CallSyncer{
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
		timeout:    timeout,
		writer:     writer,
		reducer:    reducer,
		reduceCh:   handleToChan(logger, reducer),
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

// Call fires a bunch of function callings, and wait the result of these function callings.
// The result only contains the messages with role=="tool".
// If some function callings failed, the content will be returned as the failed reason.
func (f *CallSyncer) Call(ctx context.Context, transID, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) ([]openai.ChatCompletionMessage, error) {
	if err := f.fire(transID, reqID, tagToolCalls); err != nil {
		return nil, err
	}
	// this channel should has a buffer of 1, otherwise dispatch() maybe blocked.
	ch := make(chan []openai.ChatCompletionMessage, 1)

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

func (f *CallSyncer) fire(transID string, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) error {
	toolIDs := make(map[string]struct{})

	for tag, tools := range tagToolCalls {
		f.logger.Debug("fire tool_calls", "tag", tag, "len(tools)", len(tools), "transID", transID, "reqID", reqID)

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

// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
func (f *CallSyncer) Close() error {
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

type msg struct {
	deadline time.Time
	// key is toolCallID, value is nil or the response from the toolCall.
	messages map[string]*openai.ChatCompletionMessage
}

func (f *CallSyncer) dispatch(
	reqID string,
	requests map[string]*msg,
	buffered map[string]map[string]openai.ChatCompletionMessage,
	response map[string]chan []openai.ChatCompletionMessage,
) {
	// not fired.
	req, ok := requests[reqID]
	if !ok {
		return
	}

	// deadline expired
	if req.deadline.Before(time.Now()) {
		for toolID, msg := range req.messages {
			if msg != nil {
				continue
			}
			req.messages[toolID] = &openai.ChatCompletionMessage{
				ToolCallID: toolID,
				Role:       openai.ChatMessageRoleTool,
				Content:    "timeout in this function calling, you should ignore this. ",
			}
		}
	}

	var result []openai.ChatCompletionMessage
	i := 0
	for _, msg := range req.messages {
		if msg == nil {
			f.logger.Debug("dispatch", "reqID", reqID, "fired", len(req.messages), "received", i)
			return
		}
		result = append(result, *msg)
		i++
	}

	ch, ok := response[reqID]
	if !ok {
		return
	}

	select {
	case ch <- result:
		// complete a request-response, clean up this request according to the reqID.
		delete(requests, reqID)
		delete(buffered, reqID)
		delete(response, reqID)
		f.logger.Debug("dispatch", "reqID", reqID, "fired", len(req.messages), "received", i)
	default:
	}
}

func (f *CallSyncer) background() {
	// requests stores the request that be fired, the key is the reqID
	requests := make(map[string]*msg)
	// buffered stores the messages from the reducer, the key is the reqID
	buffered := make(map[string]map[string]openai.ChatCompletionMessage)
	// response stores the result channel, the key is the reqID, the value channel will be sent when the buffered is fulled.
	response := make(map[string]chan []openai.ChatCompletionMessage)

	for {
		select {
		case <-f.ctx.Done():
			return
		case reqTools := <-f.reqToolsCh:
			item := &msg{
				deadline: time.Now().Add(f.timeout),
				messages: make(map[string]*openai.ChatCompletionMessage),
			}
			for toolID := range reqTools.toolIDs {
				item.messages[toolID] = nil
			}
			for k, v := range buffered[reqTools.reqID] {
				item.messages[k] = &openai.ChatCompletionMessage{
					ToolCallID: v.ToolCallID,
					Role:       v.Role,
					Content:    v.Content,
				}
			}
			requests[reqTools.reqID] = item

			f.logger.Debug("background request recv", "reqID", reqTools.reqID, "fired", len(item.messages), "buffered", len(buffered[reqTools.reqID]))
			f.dispatch(reqTools.reqID, requests, buffered, response)

		case rc := <-f.reqMsgChCh:
			response[rc.reqID] = rc.ch

			f.logger.Debug("background response recv", "reqID", rc.reqID, "fired", len(requests[rc.reqID].messages), "buffered", len(buffered[rc.reqID]))
			f.dispatch(rc.reqID, requests, buffered, response)

		case msg := <-f.reduceCh:
			tool, ok := requests[msg.reqID]
			if !ok {
				_, ok := buffered[msg.reqID]
				if !ok {
					buffered[msg.reqID] = make(map[string]openai.ChatCompletionMessage)
				}
				buffered[msg.reqID][msg.message.ToolCallID] = openai.ChatCompletionMessage{
					ToolCallID: msg.message.ToolCallID,
					Role:       msg.message.Role,
					Content:    msg.message.Content,
				}
				continue
			}
			tool.messages[msg.message.ToolCallID] = &msg.message

			f.logger.Debug("background buffered recv", "reqID", msg.reqID, "fired", len(requests[msg.reqID].messages), "buffered", len(buffered[msg.reqID]))
			f.dispatch(msg.reqID, requests, buffered, response)
		}
	}
}

func handleToChan(logger *slog.Logger, reducer Reducer) <-chan reqMessage {
	ch := make(chan reqMessage)

	reducer.SetHandler(func(ctx serverless.Context) {
		invoke, err := ctx.LLMFunctionCall()
		if err != nil {
			ch <- reqMessage{reqID: ""}
			logger.Error("parse function calling invoke", "err", err.Error())
			return
		}
		logger.Debug("sfn-reducer", "req_id", invoke.ReqID, "tool_call_id", invoke.ToolCallID, "result", string(invoke.Result))

		message := openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    invoke.Result,
			ToolCallID: invoke.ToolCallID,
		}

		ch <- reqMessage{reqID: invoke.ReqID, message: message}
	})

	return ch
}

type (
	// TagWriter write tag and []byte.
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
