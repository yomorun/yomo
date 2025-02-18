package ai

import (
	"context"
	"log/slog"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
)

// CallSyncer fires a bunch of function callings, and wait the result of these function callings.
// every tool call has a toolCallID, which is used to identify the function calling,
// Note that one tool call can only be responded once.
type CallSyncer interface {
	// Call fires a bunch of function callings, and wait the result of these function callings.
	// The result only contains the messages with role=="tool".
	// If some function callings failed, the content will be returned as the failed reason.
	Call(ctx context.Context, transID string, reqID string, toolCalls map[uint32][]*openai.ToolCall) ([]ToolCallResult, error)
	// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
	Close() error
}

// ToolCallResult is the result of a CallSyncer.Call()
type ToolCallResult struct {
	// ToolCallID is the tool call id.
	ToolCallID string
	// Content is the result of the function calling.
	Content string
}

type callSyncer struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	// timeout is the timeout for waiting the result.
	timeout   time.Duration
	sourceCh  chan<- TagFunctionCall
	reduceCh  <-chan ReduceMessage
	toolOutCh chan toolOut
	cleanCh   chan string
}

// ReduceMessage is the message from the reducer.
type ReduceMessage struct {
	// ReqID indentifies the message.
	ReqID string
	// Message is the message.
	Message openai.ChatCompletionMessage
}

// TagFunctionCall is the request to the syncer.
// It always be sent to the source.
type TagFunctionCall struct {
	// Tag is the tag of the request.
	Tag uint32
	// FunctionCall is the function call.
	// It cantains the arguments and the function name.
	FunctionCall *ai.FunctionCall
}

// NewCallSyncer creates a new CallSyncer.
func NewCallSyncer(logger *slog.Logger, sourceCh chan<- TagFunctionCall, reduceCh <-chan ReduceMessage, timeout time.Duration) CallSyncer {
	if timeout == 0 {
		timeout = RunFunctionTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())

	syncer := &callSyncer{
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		timeout:   timeout,
		sourceCh:  sourceCh,
		reduceCh:  reduceCh,
		toolOutCh: make(chan toolOut),
		cleanCh:   make(chan string),
	}

	go syncer.background()

	return syncer
}

type toolOut struct {
	reqID   string
	toolIDs map[string]struct{}
	ch      chan ToolCallResult
}

func (f *callSyncer) Call(ctx context.Context, transID, reqID string, tagToolCalls map[uint32][]*openai.ToolCall) ([]ToolCallResult, error) {
	defer func() {
		f.cleanCh <- reqID
	}()

	toolIDs, err := f.fire(transID, reqID, tagToolCalls)
	if err != nil {
		return nil, err
	}
	ch := make(chan ToolCallResult)

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

	var result []ToolCallResult
	for {
		select {
		case <-f.ctx.Done():
			// the  TTL of cached reducer for users reached,
			// return ctx error temporarily.
			return nil, f.ctx.Err()
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
				result = append(result, ToolCallResult{
					ToolCallID: id,
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
			f.sourceCh <- TagFunctionCall{
				Tag: tag,
				FunctionCall: &ai.FunctionCall{
					TransID:      transID,
					ReqID:        reqID,
					ToolCallID:   t.ID,
					FunctionName: t.Function.Name,
					Arguments:    t.Function.Arguments,
				},
			}
			ToolIDs[t.ID] = struct{}{}
		}
	}

	return ToolIDs, nil
}

// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
func (f *callSyncer) Close() error {
	f.cancel()
	return nil
}

func (f *callSyncer) background() {
	// buffered stores the messages from the reducer, the key is the reqID
	buffered := make(map[string]map[string]ToolCallResult)
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
			if msg.ReqID == "" {
				f.logger.Warn("recv unexpected message", "msg", msg)
				continue
			}
			result := ToolCallResult{
				ToolCallID: msg.Message.ToolCallID,
				Content:    msg.Message.Content,
			}

			sig, ok := singnals[msg.ReqID]
			// the signal that requests a result has not been sent. so buffer the data from reducer.
			if !ok {
				_, ok := buffered[msg.ReqID]
				if !ok {
					buffered[msg.ReqID] = make(map[string]ToolCallResult)
				}
				buffered[msg.ReqID][msg.Message.ToolCallID] = result
			} else {
				// the signal was sent,
				// check if the message has been sent, and if not, send the message to signal's channel.
				if _, ok := sig.toolIDs[msg.Message.ToolCallID]; ok {
					sig.ch <- result
				}
			}
		}
	}
}
