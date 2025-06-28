package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

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
	checkFunctionCall() (bool, error)
	// getToolCalls returns the tool calls in the response
	getToolCalls() []openai.ToolCall
	// getUsage returns the usage in the response
	getUsage() openai.Usage
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

	return &chatResp{resp}, nil
}

type chatContext struct {
	// callTimes is the number of times for calling chat completions
	callTimes int
	// totalUsage is the total usage of all chat completions
	totalUsage openai.Usage
	req        openai.ChatCompletionRequest
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
		// write header if it's a streaming request (write & flush header before write body)
		if req.Stream && chatCtx.callTimes == 0 {
			w.SetStreamHeader()
			w.Flush()
		}

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

		isFunctionCall, err := resp.checkFunctionCall()
		if err != nil {
			reqSpan.RecordError(err)
			reqSpan.End()
			return err
		}
		if isFunctionCall {
			callCtx, callSpan := tracer.Start(ctx, fmt.Sprintf("call_functions(#%d)", chatCtx.callTimes+1))
			toolCalls := resp.getToolCalls()

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

			updateTotalUsage(chatCtx, resp)

			chatCtx.callTimes++
			continue
		} else {
			reqSpan.End()
			return endCall(gctx, chatCtx, resp, w, tracer)
		}
	}
}

func endCall(ctx context.Context, chatCtx *chatContext, resp chatResponse, w EventResponseWriter, tracer trace.Tracer) error {
	updateTotalUsage(chatCtx, resp)

	_, respSpan := tracer.Start(ctx, "chat_completions_response")
	defer respSpan.End()

	if err := resp.writeResponse(w, chatCtx); err != nil {
		respSpan.RecordError(err)
		return err
	}

	return nil
}

func updateTotalUsage(chatCtx *chatContext, resp chatResponse) {
	usage := resp.getUsage()
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

func (c *chatResp) checkFunctionCall() (bool, error) {
	isFunctionCall := c.resp.Choices[0].FinishReason == openai.FinishReasonToolCalls ||
		len(c.resp.Choices[0].Message.ToolCalls) != 0
	return isFunctionCall, nil
}

func (c *chatResp) getToolCalls() []openai.ToolCall {
	return c.resp.Choices[0].Message.ToolCalls
}

func (c *chatResp) getUsage() openai.Usage {
	return c.resp.Usage
}

func (c *chatResp) writeResponse(w EventResponseWriter, chatCtx *chatContext) error {
	// accumulate usage before responding, so use `=` rather than `+=`
	c.resp.Usage = chatCtx.totalUsage

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(c.resp)
}

// streamChatResp is the streaming implementation of chatResponse
type streamChatResp struct {
	// buffer is the response be buffered before check if it is a function call
	buffer       []openai.ChatCompletionStreamResponse
	recver       provider.ResponseRecver
	usage        openai.Usage
	toolCallsMap map[int]openai.ToolCall
}

var _ chatResponse = (*streamChatResp)(nil)

func (resp *streamChatResp) checkFunctionCall() (bool, error) {
	var role string

	for {
		delta, err := resp.recver.Recv()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		choices := delta.Choices

		// return when choices is not empty
		if len(choices) > 0 {
			resp.buffer = append(resp.buffer, delta)
			if r := choices[0].Delta.Role; r != "" {
				role = r
			}
			if role == openai.ChatMessageRoleAssistant && len(choices[0].Delta.ToolCalls) > 0 {
				return true, nil
			}
			if role == openai.ChatMessageRoleAssistant && len(choices[0].Delta.ReasoningContent) != 0 {
				return false, nil
			}
			if role == openai.ChatMessageRoleAssistant && len(choices[0].Delta.Content) != 0 {
				return false, nil
			}
			continue
		}
	}
}

func (r *streamChatResp) getToolCalls() []openai.ToolCall {
	for _, resp := range r.buffer {
		if len(resp.Choices) > 0 {
			r.accuamulate(resp.Choices[0].Delta.ToolCalls)
		}
	}

	for {
		resp, err := r.recver.Recv()
		if err != nil {
			break
		}

		if resp.Usage != nil {
			r.usage = *resp.Usage
		}

		if len(resp.Choices) > 0 {
			r.accuamulate(resp.Choices[0].Delta.ToolCalls)
		}
	}

	toolCalls := make([]openai.ToolCall, len(r.toolCallsMap))
	for k, v := range r.toolCallsMap {
		toolCalls[k] = v
	}
	return toolCalls
}

func (r *streamChatResp) getUsage() openai.Usage {
	return r.usage
}

func (s *streamChatResp) writeResponse(w EventResponseWriter, chatCtx *chatContext) error {
	for _, resp := range s.buffer {
		if err := w.WriteStreamEvent(resp); err != nil {
			return err
		}
	}

	for {
		resp, err := s.recver.Recv()
		if err != nil {
			if err == io.EOF {
				w.WriteStreamDone()
				return nil
			}
			return err
		}
		if len(resp.PromptFilterResults) > 0 {
			continue
		}
		// response total usage when usage is not nil
		if resp.Usage != nil {
			resp.Usage = &chatCtx.totalUsage
		}
		if err := w.WriteStreamEvent(resp); err != nil {
			return err
		}
	}
}

func (r *streamChatResp) accuamulate(delta []openai.ToolCall) {
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

func (i *invokeResp) checkFunctionCall() (bool, error) { return i.underlying.checkFunctionCall() }
func (i *invokeResp) getToolCalls() []openai.ToolCall  { return i.underlying.getToolCalls() }
func (i *invokeResp) getUsage() openai.Usage           { return i.underlying.getUsage() }

func newInvokeResp(resp openai.ChatCompletionResponse, includeCallStack bool) *invokeResp {
	return &invokeResp{
		underlying:       &chatResp{resp},
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
