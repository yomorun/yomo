package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel/trace"
)

// chatResponse defines how to extract data from the response of chat completions and how to return the response.
// chatResponse is to abstract both streaming and non-streaming requests, making them clearer within multiTurnFunctionCall.
type chatResponse interface {
	// checkFunctionCall checks if the response is a function call response
	checkFunctionCall(w EventResponseWriter, chatCtx *chatContext) (bool, error)
	// getToolCalls returns the tool calls and its usage in the response
	getToolCalls() ([]openai.ToolCall, openai.Usage)
	// writeResponse defines how to write the response to the response writer
	writeResponse(w EventResponseWriter, chatCtx *chatContext) error
}

const (
	invokeMetadataKey                 = "invoke"
	invokeIncludeCallStackMetadataKey = "invoke_include_call_stack"
)

// createChatCompletions creates a chat completions response.
func createChatCompletions(ctx context.Context, p provider.LLMProvider, req openai.ChatCompletionRequest, md metadata.M) (chatResponse, error) {
	if req.Stream {
		stream, err := p.GetChatCompletionsStream(ctx, req, md)
		if err != nil {
			return nil, err
		}
		resp := &streamChatResp{
			recver:       stream,
			toolCallsMap: make(map[int]openai.ToolCall),
		}

		return resp, nil
	}

	resp, err := p.GetChatCompletions(ctx, req, md)
	if err != nil {
		return nil, err
	}

	if _, ok := md.Get(invokeMetadataKey); ok {
		if val, _ := md.Get(invokeIncludeCallStackMetadataKey); val == "true" {
			return newInvokeResp(resp, true), nil
		} else {
			return newInvokeResp(resp, false), nil
		}
	}

	return &chatResp{resp: resp}, nil
}

type chatContext struct {
	id string
	// callTimes is the number of times for calling chat completions
	callTimes int
	// totalUsage is the total usage of all chat completions (include tool calls and final response)
	totalUsage openai.Usage
	// req.Messages is the chat history
	req openai.ChatCompletionRequest
}

// multiTurnFunctionCalling calls chat completions multiple times until finishing function calling
func multiTurnFunctionCalling(
	gctx context.Context,
	req openai.ChatCompletionRequest,
	transID string,
	hasReqTools bool,
	w EventResponseWriter,
	p provider.LLMProvider,
	caller *Caller,
	tracer trace.Tracer,
	md metadata.M,
) error {
	var (
		maxCalls = 14
		chatCtx  = &chatContext{req: req}
	)

	ctx, reqSpan := tracer.Start(gctx, "chat_completions_request")
	for {
		// second and later calls should not have tool_choice option
		if chatCtx.callTimes != 0 {
			chatCtx.req.ToolChoice = nil
		}

		reqCtx, chatSpan := tracer.Start(ctx, fmt.Sprintf("llm_chat(#%d)", chatCtx.callTimes+1))
		resp, err := createChatCompletions(reqCtx, p, chatCtx.req, md)
		if err != nil {
			chatSpan.RecordError(err)
			chatSpan.End()
			reqSpan.End()
			return err
		}
		chatSpan.End()

		// write header if it's a streaming request (write & flush header before write body)
		if req.Stream && chatCtx.callTimes == 0 {
			w.SetStreamHeader()
			w.Flush()
		}

		// return the response if it's the last call
		if chatCtx.callTimes == maxCalls {
			reqSpan.End()
			return endCall(gctx, chatCtx, resp, w, tracer)
		}

		// if the request contains tools, return the response directly
		if hasReqTools {
			reqSpan.End()
			return resp.writeResponse(w, chatCtx)
		}

		isFunctionCall, err := resp.checkFunctionCall(w, chatCtx)
		if err != nil {
			reqSpan.RecordError(err)
			reqSpan.End()
			return err
		}
		if isFunctionCall {
			callCtx, callSpan := tracer.Start(ctx, fmt.Sprintf("call_functions(#%d)", chatCtx.callTimes+1))
			toolCalls, usage := resp.getToolCalls()

			// add toolCallID if toolCallID is empty
			for i, call := range toolCalls {
				if call.ID == "" {
					toolCalls[i].ID = fmt.Sprintf("%s_%d", call.Function.Name, i)
				}
			}
			// append role=assistant (argeuments) to context
			chatCtx.req.Messages = append(chatCtx.req.Messages, openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			})

			// call functions
			reqID := id.New(16)
			callResult, err := caller.Call(callCtx, transID, reqID, toolCalls, tracer)
			if err != nil {
				callSpan.RecordError(err)
				callSpan.End()
				reqSpan.End()
				return err
			}
			if req.Stream {
				_ = w.WriteStreamEvent(toolCalls)
			}
			callSpan.End()

			// append role=tool (call result) to context
			for _, call := range callResult {
				chatCtx.req.Messages = append(chatCtx.req.Messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: call.ToolCallID,
					Content:    call.Content,
				})
			}

			updateCtxUsage(chatCtx, usage)

			chatCtx.callTimes++
			continue
		} else {
			reqSpan.End()
			return endCall(gctx, chatCtx, resp, w, tracer)
		}
	}
}

func endCall(ctx context.Context, chatCtx *chatContext, resp chatResponse, w EventResponseWriter, tracer trace.Tracer) error {
	_, respSpan := tracer.Start(ctx, "chat_completions_response")
	defer respSpan.End()

	if err := resp.writeResponse(w, chatCtx); err != nil {
		respSpan.RecordError(err)
		return err
	}

	return nil
}

func updateCtxUsage(chatCtx *chatContext, usage openai.Usage) {
	chatCtx.totalUsage.PromptTokens += usage.PromptTokens
	chatCtx.totalUsage.CompletionTokens += usage.CompletionTokens
	chatCtx.totalUsage.TotalTokens += usage.TotalTokens

	if detail := usage.PromptTokensDetails; detail != nil {
		if chatCtx.totalUsage.PromptTokensDetails == nil {
			chatCtx.totalUsage.PromptTokensDetails = detail
		} else {
			chatCtx.totalUsage.PromptTokensDetails.CachedTokens += detail.CachedTokens
			chatCtx.totalUsage.PromptTokensDetails.AudioTokens += detail.AudioTokens
		}
	}
	if detail := usage.CompletionTokensDetails; detail != nil {
		if chatCtx.totalUsage.CompletionTokensDetails == nil {
			chatCtx.totalUsage.CompletionTokensDetails = detail
		} else {
			chatCtx.totalUsage.CompletionTokensDetails.AudioTokens += detail.AudioTokens
			chatCtx.totalUsage.CompletionTokensDetails.ReasoningTokens += detail.ReasoningTokens
			chatCtx.totalUsage.CompletionTokensDetails.AcceptedPredictionTokens += detail.AcceptedPredictionTokens
			chatCtx.totalUsage.CompletionTokensDetails.RejectedPredictionTokens += detail.RejectedPredictionTokens
		}
	}
}

// chatResp is the non-streaming implementation of chatResponse
type chatResp struct {
	resp openai.ChatCompletionResponse
}

var _ chatResponse = &chatResp{}

func (c *chatResp) checkFunctionCall(_ EventResponseWriter, chatCtx *chatContext) (bool, error) {
	if len(c.resp.Choices) == 0 {
		return false, nil
	}
	isFunctionCall := c.resp.Choices[0].FinishReason == openai.FinishReasonToolCalls ||
		len(c.resp.Choices[0].Message.ToolCalls) != 0
	return isFunctionCall, nil
}

func (c *chatResp) getToolCalls() ([]openai.ToolCall, openai.Usage) {
	originalToolCalls := c.resp.Choices[0].Message.ToolCalls
	copiedToolCalls := make([]openai.ToolCall, len(originalToolCalls))
	copy(copiedToolCalls, originalToolCalls)

	return copiedToolCalls, c.resp.Usage
}

func (c *chatResp) writeResponse(w EventResponseWriter, chatCtx *chatContext) error {
	updateCtxUsage(chatCtx, c.resp.Usage)
	c.resp.Usage = chatCtx.totalUsage

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(c.resp)
}

// streamChatResp is the streaming implementation of chatResponse
type streamChatResp struct {
	recver       provider.ResponseRecver
	finishReason openai.FinishReason
	// buffer is the buffer of chunks that contain no content
	buffer []openai.ChatCompletionStreamResponse
	// toolCallDeltas is the delta responses that contain tool calls
	toolCallDeltas []openai.ChatCompletionStreamResponse
	toolCallsMap   map[int]openai.ToolCall
}

var _ chatResponse = (*streamChatResp)(nil)

func (r *streamChatResp) checkFunctionCall(w EventResponseWriter, chatCtx *chatContext) (bool, error) {
	for {
		isFunctionCall := r.finishReason == openai.FinishReasonToolCalls || r.finishReason == "tool_call"

		chunk, err := r.recver.Recv()
		if err == io.EOF {
			return isFunctionCall, nil
		}
		if err != nil {
			return isFunctionCall, err
		}

		if chunk.ID == "" {
			continue
		}
		if chatCtx.id == "" {
			chatCtx.id = chunk.ID
		}

		if usage := chunk.Usage; usage != nil {
			updateCtxUsage(chatCtx, *usage)
		}

		if len(chunk.Choices) == 0 {
			if err := r.writeEvent(w, chunk, chatCtx); err != nil {
				return false, err
			}
			continue
		}

		choice := chunk.Choices[0]

		if chunk.Choices[0].FinishReason != "" {
			r.finishReason = chunk.Choices[0].FinishReason
		}

		// no content chunk (role chunk), just buffer it
		if choice.Delta.Content == "" && choice.Delta.ReasoningContent == "" && len(choice.Delta.ToolCalls) == 0 {
			r.buffer = append(r.buffer, chunk)
			continue
		}

		if len(choice.Delta.ToolCalls) != 0 {
			r.toolCallDeltas = append(r.toolCallDeltas, chunk)
			if choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" {
				// just response content and reasoning content
				chunk.Choices[0].Delta.ToolCalls = nil
				if err := r.writeEvent(w, chunk, chatCtx); err != nil {
					return false, err
				}
			}
			continue
		}
		if err := r.writeEvent(w, chunk, chatCtx); err != nil {
			return false, err
		}
	}
}

func (r *streamChatResp) getToolCalls() ([]openai.ToolCall, openai.Usage) {
	usage := openai.Usage{}

	for _, resp := range r.toolCallDeltas {
		if len(resp.Choices) > 0 {
			r.accuamulateToolCall(resp.Choices[0].Delta.ToolCalls)
		}
	}

	toolCalls := make([]openai.ToolCall, 0, len(r.toolCallsMap))
	for _, v := range r.toolCallsMap {
		toolCalls = append(toolCalls, v)
	}

	slices.SortFunc(toolCalls, func(i, j openai.ToolCall) int {
		var iIndex, jIndex int
		if i.Index != nil {
			iIndex = *i.Index
		}
		if j.Index != nil {
			jIndex = *j.Index
		}
		return iIndex - jIndex
	})

	return toolCalls, usage
}

func (r *streamChatResp) writeResponse(w EventResponseWriter, chatCtx *chatContext) error {
	return w.WriteStreamDone()
}

func (r *streamChatResp) writeEvent(w EventResponseWriter, chunk openai.ChatCompletionStreamResponse, chatCtx *chatContext) error {
	chunks := append(r.buffer, chunk)

	defer func() {
		r.buffer = r.buffer[:0]
	}()

	for _, v := range chunks {
		if len(v.Choices) > 0 && v.Choices[0].FinishReason == openai.FinishReasonFunctionCall {
			return nil
		}
		if v.Usage != nil {
			v.Usage = &chatCtx.totalUsage
		}
		if r.finishReason != openai.FinishReasonToolCalls {
			v.ID = chatCtx.id
			if err := w.WriteStreamEvent(v); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *streamChatResp) accuamulateToolCall(delta []openai.ToolCall) {
	for k, v := range delta {
		index := k
		if v.Index != nil {
			index = *v.Index
		}
		item, ok := r.toolCallsMap[index]
		if !ok {
			r.toolCallsMap[index] = openai.ToolCall{
				Index:    v.Index,
				ID:       v.ID,
				Type:     v.Type,
				Function: openai.FunctionCall{},
			}
			item = r.toolCallsMap[index]
		}
		if v.Function.Arguments != "" {
			item.Function.Arguments += v.Function.Arguments
		}
		if v.Function.Name != "" {
			item.Function.Name = v.Function.Name
		}
		r.toolCallsMap[index] = item
	}
}

type invokeResp struct {
	underlying       *chatResp
	includeCallStack bool
}

var _ chatResponse = (*invokeResp)(nil)

func (i *invokeResp) checkFunctionCall(w EventResponseWriter, chatCtx *chatContext) (bool, error) {
	return i.underlying.checkFunctionCall(w, chatCtx)
}

func (i *invokeResp) getToolCalls() ([]openai.ToolCall, openai.Usage) {
	return i.underlying.getToolCalls()
}

func newInvokeResp(resp openai.ChatCompletionResponse, includeCallStack bool) *invokeResp {
	return &invokeResp{
		underlying:       &chatResp{resp: resp},
		includeCallStack: includeCallStack,
	}
}

func (i *invokeResp) writeResponse(w EventResponseWriter, chatCtx *chatContext) error {
	resp := ai.InvokeResponse{
		Content:      i.underlying.resp.Choices[0].Message.Content,
		FinishReason: string(i.underlying.resp.Choices[0].FinishReason),
		TokenUsage: ai.TokenUsage{
			PromptTokens:     i.underlying.resp.Usage.PromptTokens,
			CompletionTokens: i.underlying.resp.Usage.CompletionTokens,
		},
	}
	if i.includeCallStack {
		resp.History = chatCtx.req.Messages
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}
