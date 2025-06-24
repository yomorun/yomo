package ai

import (
	"context"
	"log/slog"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel/trace"
)

// CallSyncer fires a bunch of function callings, and wait the result of these function callings.
// every tool call has a toolCallID, which is used to identify the function calling,
// Note that one tool call can only be responded once.
type CallSyncer interface {
	// Call fires a bunch of function callings, and wait the result of these function callings.
	// If some function callings failed, the content will be returned as the failed reason.
	Call(ctx context.Context, transID string, reqID string, toolCalls []openai.ToolCall, tracer trace.Tracer) ([]ToolCallResult, error)
	// Close close the CallSyncer. if close, you can't use this CallSyncer anymore.
	Close() error
}

// ToolCallResult is the result of a CallSyncer.Call()
type ToolCallResult struct {
	// ReqID identifies the tool call result.
	ReqID string
	// FunctionName is the name of the function calling.
	FunctionName string
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
	sourceCh  chan<- ai.FunctionCall
	reduceCh  <-chan ToolCallResult
	toolOutCh chan toolOut
	cleanCh   chan string
}

// NewCallSyncer creates a new CallSyncer.
func NewCallSyncer(logger *slog.Logger, sourceCh chan<- ai.FunctionCall, reduceCh <-chan ToolCallResult, timeout time.Duration) CallSyncer {
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

type callSpan struct {
	hasID bool
	name  string
	span  trace.Span
}

func (f *callSyncer) Call(ctx context.Context, transID, reqID string, toolCalls []openai.ToolCall, tracer trace.Tracer) ([]ToolCallResult, error) {
	if len(toolCalls) == 0 {
		return []ToolCallResult{}, nil
	}
	defer func() {
		f.cleanCh <- reqID
	}()

	var (
		callSpans     = make(map[string]callSpan)
		toolCallsCopy = make([]openai.ToolCall, len(toolCalls)) // toolCalls
	)

	for i, tool := range toolCalls {
		toolCallsCopy[i] = tool
		var (
			hasID  = true
			toolID = tool.ID
		)
		// support toolCallID="" or toolCallID=functionName (gemini & vertexai compatible),
		// if toolCallID is empty, ToolCallResult.ToolCallID will be the functionName
		if tool.ID == "" || tool.ID == tool.Function.Name {
			hasID = false
			toolID = id.New(8)
			toolCallsCopy[i].ID = toolID
		}
		_, span := tracer.Start(ctx, tool.Function.Name)
		callSpans[toolID] = callSpan{
			hasID: hasID,
			span:  span,
			name:  tool.Function.Name,
		}
	}

	toolIDs, err := f.fire(transID, reqID, toolCallsCopy)
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
			callSpan := callSpans[res.ToolCallID]
			callSpan.span.End()

			delete(toolIDs, res.ToolCallID)

			if !callSpan.hasID {
				res.ToolCallID = res.FunctionName
			}
			result = append(result, res)

			if len(toolIDs) == 0 {
				return result, nil
			}
		case <-time.After(f.timeout):
			for toolID := range toolIDs {
				callSpan := callSpans[toolID]

				toolCallID := toolID
				if !callSpan.hasID {
					toolCallID = callSpan.name
				}
				result = append(result, ToolCallResult{
					FunctionName: callSpan.name,
					ToolCallID:   toolCallID,
					Content:      "timeout in this function calling, you should ignore this.",
				})

				callSpan.span.End()
			}
			return result, nil
		}
	}
}

func (f *callSyncer) fire(transID string, reqID string, toolCalls []openai.ToolCall) (map[string]struct{}, error) {
	ToolIDs := make(map[string]struct{})

	f.logger.Debug("fire tool_calls", "transID", transID, "reqID", reqID, "len(tool_calls)", len(toolCalls))

	for _, t := range toolCalls {
		f.sourceCh <- ai.FunctionCall{
			TransID:      transID,
			ReqID:        reqID,
			ToolCallID:   t.ID,
			FunctionName: t.Function.Name,
			Arguments:    t.Function.Arguments,
		}
		ToolIDs[t.ID] = struct{}{}
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
				FunctionName: msg.FunctionName,
				ToolCallID:   msg.ToolCallID,
				Content:      msg.Content,
			}

			sig, ok := singnals[msg.ReqID]
			// the signal that requests a result has not been sent. so buffer the data from reducer.
			if !ok {
				_, ok := buffered[msg.ReqID]
				if !ok {
					buffered[msg.ReqID] = make(map[string]ToolCallResult)
				}
				buffered[msg.ReqID][msg.ToolCallID] = result
			} else {
				// the signal was sent,
				// check if the message has been sent, and if not, send the message to signal's channel.
				if _, ok := sig.toolIDs[msg.ToolCallID]; ok {
					sig.ch <- result
				}
			}
		}
	}
}
