package ai

import (
	"context"
	"log/slog"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/serverless"
)

// CallSyncer fires a bunch of function callings, and wait the result of these function callings.
// every tool call has a toolCallID, which is used to identify the function calling,
// Note that one tool call can only be responded once.
type CallSyncer interface {
	// Call fires a bunch of function callings, and wait the result of these function callings.
	// The result only contains the messages with role=="tool".
	// If some function callings failed, the content will be returned as the failed reason.
	Call(ctx context.Context, transID string, reqID string, toolCalls map[uint32][]*openai.ToolCall) ([]openai.ChatCompletionMessage, error)
	// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
	Close() error
}

type callSyncer struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	// timeout is the timeout for waiting the result.
	timeout   time.Duration
	source    yomo.Source
	reducer   yomo.StreamFunction
	reduceCh  <-chan refucerMessage
	toolOutCh chan toolOut
	cleanCh   chan string
}

type refucerMessage struct {
	reqID   string
	message openai.ChatCompletionMessage
}

// NewCallSyncer creates a new CallSyncer.
func NewCallSyncer(logger *slog.Logger, source yomo.Source, reducer yomo.StreamFunction, timeout time.Duration) CallSyncer {
	if timeout == 0 {
		timeout = RunFunctionTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())

	syncer := &callSyncer{
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		timeout:   timeout,
		source:    source,
		reducer:   reducer,
		reduceCh:  handleToChan(logger, reducer),
		toolOutCh: make(chan toolOut),
		cleanCh:   make(chan string),
	}

	go syncer.background()

	return syncer
}

type toolOut struct {
	reqID   string
	toolIDs map[string]struct{}
	ch      chan openai.ChatCompletionMessage
}

func (f *callSyncer) Call(ctx context.Context, transID, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) ([]openai.ChatCompletionMessage, error) {
	defer func() {
		f.cleanCh <- reqID
	}()

	toolIDs, err := f.fire(transID, reqID, tagToolCalls)
	if err != nil {
		return nil, err
	}
	ch := make(chan openai.ChatCompletionMessage)

	otherToolIDs := make(map[string]struct{})
	for id := range toolIDs {
		otherToolIDs[id] = struct{}{}
	}

	toolOut := toolOut{
		reqID:   reqID,
		toolIDs: otherToolIDs,
		ch:      ch,
	}

	f.toolOutCh <- toolOut

	var result []openai.ChatCompletionMessage
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-ch:
			result = append(result, res)

			delete(toolIDs, res.ToolCallID)
			if len(toolIDs) == 0 {
				return result, nil
			}
		case <-time.After(f.timeout):
			for id := range toolIDs {
				result = append(result, openai.ChatCompletionMessage{
					ToolCallID: id,
					Role:       openai.ChatMessageRoleTool,
					Content:    "timeout in this function calling, you should ignore this.",
				})
			}
			return result, nil
		}
	}
}

func (f *callSyncer) fire(transID string, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) (map[string]struct{}, error) {
	ToolIDs := make(map[string]struct{})

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

			if err := f.source.Write(tag, buf); err != nil {
				// TODO: maybe we should make a send failed collection here.
				f.logger.Error("send data to zipper", "err", err.Error())
				continue
			}
			ToolIDs[t.ID] = struct{}{}
		}
	}

	return ToolIDs, nil
}

// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
func (f *callSyncer) Close() error {
	f.cancel()

	var err error
	if err = f.source.Close(); err != nil {
		f.logger.Error("callSyncer writer close", "err", err.Error())
	}

	if err = f.reducer.Close(); err != nil {
		f.logger.Error("callSyncer reducer close", "err", err.Error())
	}

	return err
}

func (f *callSyncer) background() {
	// buffered stores the messages from the reducer, the key is the reqID
	buffered := make(map[string]map[string]openai.ChatCompletionMessage)
	// singnals stores the result channel, the key is the reqID, the value channel will be sent when the buffered is fulled.
	singnals := make(map[string]toolOut)

	for {
		select {
		case <-f.ctx.Done():
			return

		case out := <-f.toolOutCh:
			singnals[out.reqID] = out

			// send data buffered to the result channel, one ToolCallID has one result.
			for _, msg := range buffered[out.reqID] {
				if _, ok := out.toolIDs[msg.ToolCallID]; !ok {
					continue
				}
				out.ch <- msg
				delete(buffered[out.reqID], msg.ToolCallID)
				delete(singnals[out.reqID].toolIDs, msg.ToolCallID)
			}

		case reqID := <-f.cleanCh:
			delete(buffered, reqID)
			delete(singnals, reqID)

		case msg := <-f.reduceCh:
			if msg.reqID == "" {
				f.logger.Warn("recv unexpected message", "msg", msg)
				continue
			}
			result := openai.ChatCompletionMessage{
				ToolCallID: msg.message.ToolCallID,
				Role:       msg.message.Role,
				Content:    msg.message.Content,
			}

			sig, ok := singnals[msg.reqID]
			// the signal that requests a result has not been sent. so buffer the data from reducer.
			if !ok {
				_, ok := buffered[msg.reqID]
				if !ok {
					buffered[msg.reqID] = make(map[string]openai.ChatCompletionMessage)
				}
				buffered[msg.reqID][msg.message.ToolCallID] = result
			} else {
				// the signal was sent,
				// check if the message has been sent, and if not, send the message to signal's channel.
				if _, ok := sig.toolIDs[msg.message.ToolCallID]; ok {
					sig.ch <- result
				}
			}
		}
	}
}

func handleToChan(logger *slog.Logger, reducer yomo.StreamFunction) <-chan refucerMessage {
	ch := make(chan refucerMessage)

	reducer.SetHandler(func(ctx serverless.Context) {
		invoke, err := ctx.LLMFunctionCall()
		if err != nil {
			ch <- refucerMessage{reqID: ""}
			logger.Error("parse function calling invoke", "err", err.Error())
			return
		}
		logger.Debug("sfn-reducer", "req_id", invoke.ReqID, "tool_call_id", invoke.ToolCallID, "result", string(invoke.Result))

		message := openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    invoke.Result,
			ToolCallID: invoke.ToolCallID,
		}

		ch <- refucerMessage{reqID: invoke.ReqID, message: message}
	})

	return ch
}
