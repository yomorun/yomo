package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	openai "github.com/sashabaranov/go-openai"
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
	writeResponse(w EventResponseWriter, usage openai.Usage) error
}

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
	ctx context.Context, req openai.ChatCompletionRequest, transID string, hasReqTools bool, w EventResponseWriter, p provider.LLMProvider, caller *Caller, tracer trace.Tracer,
) error {
	var (
		maxCalls = 10
		chatCtx  = &chatContext{req: req}
	)

	ctx, span := tracer.Start(ctx, "chat_completions_request")
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

		reqCtx, reqSpan := tracer.Start(ctx, fmt.Sprintf("request(#%d)", chatCtx.callTimes+1))
		resp, err := createChatCompletions(reqCtx, p, chatCtx.req, caller.md)
		if err != nil {
			reqSpan.RecordError(err)
			return err
		}
		reqSpan.End()

		// return the response if it's the last call
		if chatCtx.callTimes == maxCalls {
			return endCall(ctx, span, chatCtx, resp, w, tracer)
		}

		// if the request contains tools, return the response directly
		if hasReqTools {
			return resp.writeResponse(w, chatCtx.totalUsage)
		}

		isFunctionCall, err := resp.checkFunctionCall()
		if err != nil {
			return err
		}
		if isFunctionCall {
			_, toolSpan := tracer.Start(ctx, fmt.Sprintf("get_tool_calls(#%d)", chatCtx.callTimes+1))
			toolCalls := resp.getToolCalls()
			toolSpan.End()

			// append role=assistant (argeuments) to context
			chatCtx.req.Messages = append(chatCtx.req.Messages, openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			})

			// call functions
			callCtx, callSpan := tracer.Start(ctx, fmt.Sprintf("call_functions(#%d)", chatCtx.callTimes+1))
			reqID := id.New(16)
			callResult, err := caller.Call(callCtx, transID, reqID, toolCalls, tracer)
			if err != nil {
				callSpan.RecordError(err)
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
			return endCall(ctx, span, chatCtx, resp, w, tracer)
		}
	}
}

func endCall(ctx context.Context, reqSpan trace.Span, chatCtx *chatContext, resp chatResponse, w EventResponseWriter, tracer trace.Tracer) error {
	updateTotalUsage(chatCtx, resp)

	// end request span beofore responsing
	reqSpan.End()

	_, respSpan := tracer.Start(ctx, "chat_completions_response")
	if err := resp.writeResponse(w, chatCtx.totalUsage); err != nil {
		respSpan.RecordError(err)
		return err
	}
	respSpan.End()

	return nil
}

func updateTotalUsage(chatCtx *chatContext, resp chatResponse) {
	usage := resp.getUsage()
	chatCtx.totalUsage.PromptTokens += usage.PromptTokens
	chatCtx.totalUsage.CompletionTokens += usage.CompletionTokens
	chatCtx.totalUsage.TotalTokens += usage.TotalTokens

	if detail := usage.PromptTokensDetails; detail != nil {
		chatCtx.totalUsage.PromptTokensDetails.CachedTokens += detail.CachedTokens
		chatCtx.totalUsage.PromptTokensDetails.AudioTokens += detail.AudioTokens
	}
	if detail := usage.CompletionTokensDetails; detail != nil {
		chatCtx.totalUsage.CompletionTokensDetails.AudioTokens += detail.AudioTokens
		chatCtx.totalUsage.CompletionTokensDetails.ReasoningTokens += detail.ReasoningTokens
		chatCtx.totalUsage.CompletionTokensDetails.AcceptedPredictionTokens += detail.AcceptedPredictionTokens
		chatCtx.totalUsage.CompletionTokensDetails.RejectedPredictionTokens += detail.RejectedPredictionTokens
	}
}

// chatResp is the non-streaming implementation of chatResponse
type chatResp struct {
	resp openai.ChatCompletionResponse
}

var _ chatResponse = &chatResp{}

func (c *chatResp) checkFunctionCall() (bool, error) {
	isFunctionCall := c.resp.Choices[0].FinishReason == openai.FinishReasonToolCalls
	return isFunctionCall, nil
}

func (c *chatResp) getToolCalls() []openai.ToolCall {
	return c.resp.Choices[0].Message.ToolCalls
}

func (c *chatResp) getUsage() openai.Usage {
	return c.resp.Usage
}

func (c *chatResp) writeResponse(w EventResponseWriter, usage openai.Usage) error {
	c.resp.Usage = usage
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(c.resp)
}

// streamChatResp is the streaming implementation of chatResponse
type streamChatResp struct {
	roleMessage  openai.ChatCompletionStreamResponse
	head         openai.ChatCompletionStreamResponse
	recver       provider.ResponseRecver
	usage        openai.Usage
	toolCallsMap map[int]openai.ToolCall
}

var _ chatResponse = (*streamChatResp)(nil)

func (resp *streamChatResp) checkFunctionCall() (bool, error) {
	for {
		delta, err := resp.recver.Recv()
		if err != nil {
			return false, err
		}

		choices := delta.Choices

		// return when choices is not empty
		if len(choices) > 0 {
			// sometimes the first choice only contains role=assistant, so save it and continue
			if choices[0].Delta.Role != "" && len(choices[0].Delta.ToolCalls) == 0 {
				resp.roleMessage = delta
				continue
			}
			isFunctionCall := len(choices[0].Delta.ToolCalls) > 0

			resp.head = delta
			return isFunctionCall, nil
		}
	}
}

func (r *streamChatResp) getToolCalls() []openai.ToolCall {
	choices := r.head.Choices
	if len(choices) > 0 {
		r.accuamulate(choices[0].Delta.ToolCalls)
	}

	for {
		resp, err := r.recver.Recv()
		if err != nil {
			break
		}

		if resp.Usage != nil {
			r.usage.PromptTokens = resp.Usage.PromptTokens
			r.usage.CompletionTokens = resp.Usage.CompletionTokens
			r.usage.TotalTokens = resp.Usage.TotalTokens
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

func (s *streamChatResp) writeResponse(w EventResponseWriter, totalUsage openai.Usage) error {
	if err := w.WriteStreamEvent(s.roleMessage); err != nil {
		return err
	}

	if err := w.WriteStreamEvent(s.head); err != nil {
		return err
	}

	for {
		resp, err := s.recver.Recv()
		// time.Sleep(time.Millisecond * 100)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(resp.PromptFilterResults) > 0 {
			continue
		}
		// response total usage when usage is not nil
		if resp.Usage != nil {
			resp.Usage = &totalUsage
		}
		if err := w.WriteStreamEvent(resp); err != nil {
			return err
		}
	}
}

func (r *streamChatResp) accuamulate(delta []openai.ToolCall) {
	for _, t := range delta {
		index := *t.Index
		item, ok := r.toolCallsMap[index]
		if !ok {
			r.toolCallsMap[index] = openai.ToolCall{
				Index:    t.Index,
				ID:       t.ID,
				Type:     t.Type,
				Function: openai.FunctionCall{},
			}
			item = r.toolCallsMap[index]
		}
		if t.Function.Arguments != "" {
			item.Function.Arguments += t.Function.Arguments
		}
		if t.Function.Name != "" {
			item.Function.Name = t.Function.Name
		}
		r.toolCallsMap[index] = item
	}
}
