package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/caller"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// chatResponse defines how to extract data from the response of chat completions and how to return the response.
// chatResponse is to abstract both streaming and non-streaming requests, making them clearer within multiTurnFunctionCall.
type chatResponse interface {
	// process needs to implement the following:
	// 1. determine whether it is a function call
	// 2. if it is a function call, return ToolCalls and ToolCallUsage
	// 3. if it is not a function call, write the response
	process(w EventResponseWriter, chatCtx *chatContext) (*processResult, error)
	// writeResponse defines how to directly write the response
	writeResponse(w EventResponseWriter, chatCtx *chatContext) error
}

type processResult struct {
	isFunctionCall bool
	ToolCalls      []openai.ToolCall
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
	// toolSources maps tool names to their sources (true for server, false for client)
	toolSources map[string]bool
}

// multiTurnFunctionCalling calls chat completions multiple times until finishing function calling
func multiTurnFunctionCalling(
	gctx context.Context,
	req openai.ChatCompletionRequest,
	transID string,
	toolSources map[string]bool,
	w EventResponseWriter,
	p provider.LLMProvider,
	caller *caller.Caller,
	tracer trace.Tracer,
	md metadata.M,
	agentContext []byte,
) error {
	var (
		maxCalls = 14
		chatCtx  = &chatContext{req: req, toolSources: toolSources}
	)

	for {
		// second and later calls should not have tool_choice option
		if chatCtx.callTimes != 0 {
			chatCtx.req.ToolChoice = nil
		}

		// Count server and client tools
		var serverToolCount, clientToolCount int
		for _, isServer := range toolSources {
			if isServer {
				serverToolCount++
			} else {
				clientToolCount++
			}
		}

		spanOptions := []trace.SpanStartOption{
			trace.WithAttributes(
				attribute.Bool("stream", req.Stream),
				attribute.Int("server_tool_count", serverToolCount),
				attribute.Int("client_tool_count", clientToolCount),
			),
		}
		if req.ResponseFormat != nil {
			spanOptions = append(spanOptions, trace.WithAttributes(
				attribute.String("response_format_type", string(req.ResponseFormat.Type)),
			))
		}

		var (
			_, chatSpan = tracer.Start(gctx, fmt.Sprintf("llm_chat(#%d)", chatCtx.callTimes+1), spanOptions...)
			_, respSpan = tracer.Start(gctx, "chat_completions_response", spanOptions...)
		)

		resp, err := createChatCompletions(gctx, p, chatCtx.req, md)
		if err != nil {
			// chatSpan.RecordError(err)
			chatSpan.End()
			return err
		}
		// return the response directly if it's the last call
		if chatCtx.callTimes == maxCalls {
			err := resp.writeResponse(w, chatCtx)
			respSpan.End()
			if err != nil {
				// respSpan.RecordError(err)
				return err
			}
			return nil
		}

		chatCtx.callTimes++

		result, err := resp.process(w, chatCtx)
		if err != nil {
			// chatSpan.RecordError(err)
			return err
		}

		if result.isFunctionCall {
			chatSpan.End()

			// Handle mixed tool calls
			continueLoop, err := handleToolCalls(gctx, chatCtx, result.ToolCalls, req.Stream, w, resp, respSpan, caller, tracer, transID, agentContext)
			if err != nil {
				return err
			}
			if continueLoop {
				continue
			}
			return nil
		} else {
			respSpan.End()
			return nil
		}
	}
}

func doToolCall(
	ctx context.Context,
	chatCtx *chatContext,
	toolCalls []openai.ToolCall,
	w EventResponseWriter,
	caller *caller.Caller,
	tracer trace.Tracer,
	reqStream bool,
	transID string,
	agentContext []byte,
) error {
	callCtx, callSpan := tracer.Start(ctx, fmt.Sprintf("call_functions(#%d)", chatCtx.callTimes))

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
	if reqStream {
		_ = w.WriteStreamEvent(toolCalls)
	}
	// call functions
	reqID := id.New(16)
	callResult, err := caller.Call(callCtx, transID, reqID, agentContext, toolCalls, tracer)
	if err != nil {
		callSpan.RecordError(err)
		callSpan.End()
		return err
	}
	if reqStream {
		_ = w.WriteStreamEvent(callResult)
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

// handleToolCalls handles tool calls based on their sources
// Returns (continueLoop, error)
// continueLoop is true if we should continue the loop (only server tools case)
// continueLoop is false if we should return (client or mixed tools case)
func handleToolCalls(
	ctx context.Context,
	chatCtx *chatContext,
	toolCalls []openai.ToolCall,
	reqStream bool,
	w EventResponseWriter,
	resp chatResponse,
	respSpan trace.Span,
	caller *caller.Caller,
	tracer trace.Tracer,
	transID string,
	agentContext []byte,
) (bool, error) {
	// Check if there are both server and client tool calls (mixed case)
	hasServerTool := false
	hasClientTool := false

	// For mixed case, only keep client tool calls
	var clientToolCalls []openai.ToolCall

	// Analyze tool calls to determine their sources
	for _, toolCall := range toolCalls {
		if isServer, exists := chatCtx.toolSources[toolCall.Function.Name]; exists {
			if isServer {
				hasServerTool = true
			} else {
				hasClientTool = true
				clientToolCalls = append(clientToolCalls, toolCall)
			}
		} else {
			// Unknown tool source, assume client tool
			hasClientTool = true
			clientToolCalls = append(clientToolCalls, toolCall)
		}
	}

	// Handle mixed tool calls: only pass client tools to client
	if hasServerTool && hasClientTool {
		// Mixed case: discard server tools, only pass client tools
		if reqStream {
			w.SetStreamHeader()
			w.Flush()
		}
		err := writeClientToolCallsResponse(w, chatCtx, resp, clientToolCalls)
		respSpan.End()
		if err != nil {
			return false, err
		}
		return false, nil
	} else if hasClientTool {
		// Only client tools: pass through to client
		if reqStream {
			w.SetStreamHeader()
			w.Flush()
		}
		err := writeClientToolCallsResponse(w, chatCtx, resp, clientToolCalls)
		respSpan.End()
		if err != nil {
			return false, err
		}
		return false, nil
	} else {
		// Only server tools: execute them
		if err := doToolCall(ctx, chatCtx, toolCalls, w, caller, tracer, reqStream, transID, agentContext); err != nil {
			return false, err
		}
		return true, nil
	}
}

func writeClientToolCallsResponse(
	w EventResponseWriter,
	chatCtx *chatContext,
	resp chatResponse,
	clientToolCalls []openai.ToolCall,
) error {
	switch r := resp.(type) {
	case *chatResp:
		filtered := r.resp
		if len(filtered.Choices) > 0 {
			filtered.Choices[0].Message.ToolCalls = clientToolCalls
		}
		updateCtxUsage(chatCtx, filtered.Usage)
		filtered.Usage = chatCtx.totalUsage

		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(filtered)
	case *streamChatResp:
		if r.toolCallsStreamed {
			return nil
		}
		return writeFilteredStreamToolCalls(w, r, clientToolCalls)
	case *invokeResp:
		return r.writeResponse(w, chatCtx)
	}

	return resp.writeResponse(w, chatCtx)
}

func writeFilteredStreamToolCalls(
	w EventResponseWriter,
	r *streamChatResp,
	clientToolCalls []openai.ToolCall,
) error {
	allowedIndexes := make(map[int]struct{}, len(clientToolCalls))
	for i, call := range clientToolCalls {
		index := i
		if call.Index != nil {
			index = *call.Index
		}
		allowedIndexes[index] = struct{}{}
	}

	var roleChunks []openai.ChatCompletionStreamResponse
	var finishChunks []openai.ChatCompletionStreamResponse
	for _, chunk := range r.buffer {
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			finishChunks = append(finishChunks, chunk)
			continue
		}
		roleChunks = append(roleChunks, chunk)
	}

	for _, chunk := range roleChunks {
		if err := w.WriteStreamEvent(chunk); err != nil {
			return err
		}
	}

	for _, chunk := range r.toolCallDeltas {
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if len(choice.Delta.ToolCalls) == 0 {
			continue
		}
		filteredCalls := make([]openai.ToolCall, 0, len(choice.Delta.ToolCalls))
		for k, call := range choice.Delta.ToolCalls {
			index := k
			if call.Index != nil {
				index = *call.Index
			}
			if _, ok := allowedIndexes[index]; ok {
				filteredCalls = append(filteredCalls, call)
			}
		}
		if len(filteredCalls) == 0 {
			continue
		}
		chunk.Choices[0].Delta.ToolCalls = filteredCalls
		if err := w.WriteStreamEvent(chunk); err != nil {
			return err
		}
	}

	for _, chunk := range finishChunks {
		if err := w.WriteStreamEvent(chunk); err != nil {
			return err
		}
	}

	return w.WriteStreamDone()
}

// chatResp is the non-streaming implementation of chatResponse
type chatResp struct {
	resp openai.ChatCompletionResponse
}

var _ chatResponse = &chatResp{}

func (c *chatResp) process(w EventResponseWriter, chatCtx *chatContext) (*processResult, error) {
	return c.processWithResponseWriteFunc(w, chatCtx, c.writeResponse)
}

func (c *chatResp) processWithResponseWriteFunc(
	w EventResponseWriter,
	chatCtx *chatContext,
	writeResponse func(EventResponseWriter, *chatContext) error,
) (*processResult, error) {
	isFunctionCall, err := c.checkFunctionCall(w)
	if err != nil {
		return nil, err
	}

	if isFunctionCall {
		toolCalls, usage := c.getToolCalls()
		updateCtxUsage(chatCtx, usage)

		result := &processResult{
			isFunctionCall: isFunctionCall,
			ToolCalls:      toolCalls,
		}
		return result, nil
	}

	result := &processResult{
		isFunctionCall: isFunctionCall,
	}

	return result, writeResponse(w, chatCtx)
}

func (c *chatResp) checkFunctionCall(_ EventResponseWriter) (bool, error) {
	if len(c.resp.Choices) == 0 {
		return false, nil
	}
	choice := c.resp.Choices[0]
	isFunctionCall := choice.FinishReason == openai.FinishReasonToolCalls ||
		len(choice.Message.ToolCalls) != 0
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
	// streamToolCalls indicates whether tool calls should be streamed to client
	streamToolCalls bool
	// allowedToolIndexes is the set of tool call indexes for client tools
	allowedToolIndexes map[int]struct{}
	// toolCallsStreamed indicates tool calls have been streamed
	toolCallsStreamed bool
}

var _ chatResponse = (*streamChatResp)(nil)

func (r *streamChatResp) endProcess(w EventResponseWriter) (*processResult, error) {
	isFunctionCall := r.finishReason == openai.FinishReasonToolCalls || r.finishReason == "tool_call"

	if !isFunctionCall {
		if err := w.WriteStreamDone(); err != nil {
			return nil, err
		}
		return &processResult{
			isFunctionCall: false,
		}, nil
	}

	if r.streamToolCalls {
		for _, chunk := range r.buffer {
			if len(chunk.Choices) == 0 || chunk.Choices[0].FinishReason == "" {
				continue
			}
			if err := w.WriteStreamEvent(chunk); err != nil {
				return nil, err
			}
		}
		if err := w.WriteStreamDone(); err != nil {
			return nil, err
		}
	}

	toolCalls := r.getToolCalls()

	result := &processResult{
		isFunctionCall: isFunctionCall,
		ToolCalls:      toolCalls,
	}

	return result, nil
}

func (r *streamChatResp) process(w EventResponseWriter, chatCtx *chatContext) (*processResult, error) {
	setHeader := false
	for {
		chunk, err := r.recver.Recv()
		if err == io.EOF {
			return r.endProcess(w)
		}
		if err != nil {
			return nil, err
		}
		if chunk.ID == "" {
			continue
		}
		// write header when receive the first chunk
		if !setHeader && chatCtx.callTimes == 1 {
			w.SetStreamHeader()
			w.Flush()
			setHeader = true
		}
		if chatCtx.id == "" {
			chatCtx.id = chunk.ID
		}
		if usage := chunk.Usage; usage != nil && usage.TotalTokens != 0 {
			updateCtxUsage(chatCtx, *usage)
		}

		if len(chunk.Choices) == 0 {
			if err := r.writeEvent(w, chunk, chatCtx); err != nil {
				return nil, err
			}
			continue
		}

		choice := chunk.Choices[0]
		if chunk.Choices[0].FinishReason != "" {
			r.finishReason = chunk.Choices[0].FinishReason
			// ignore usage in finish chunk
			chunk.Usage = nil
		}
		// no content chunk (role chunk), just buffer it
		if choice.Delta.Content == "" && choice.Delta.ReasoningContent == "" && len(choice.Delta.ToolCalls) == 0 {
			r.buffer = append(r.buffer, chunk)
			continue
		}
		if len(choice.Delta.ToolCalls) != 0 {
			r.toolCallDeltas = append(r.toolCallDeltas, chunk)
			r.accumulateToolCall(choice.Delta.ToolCalls)
			r.updateAllowedToolIndexes(chatCtx)
			if len(r.allowedToolIndexes) != 0 {
				r.streamToolCalls = true
			}
			if r.streamToolCalls {
				if !r.toolCallsStreamed {
					if err := r.flushBufferedNonFinishChunks(w); err != nil {
						return nil, err
					}
					r.toolCallsStreamed = true
				}
				filteredCalls := r.filterToolCalls(choice.Delta.ToolCalls)
				if len(filteredCalls) != 0 {
					chunk.Choices[0].Delta.ToolCalls = filteredCalls
					if err := w.WriteStreamEvent(chunk); err != nil {
						return nil, err
					}
				}
				continue
			}
			if choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" {
				// only response content and reasoning content
				chunk.Choices[0].Delta.ToolCalls = nil
				if err := r.writeEvent(w, chunk, chatCtx); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := r.writeEvent(w, chunk, chatCtx); err != nil {
			return nil, err
		}
	}
}

func (r *streamChatResp) writeResponse(w EventResponseWriter, chatCtx *chatContext) error {
	for {
		chunk, err := r.recver.Recv()
		if err == io.EOF {
			return w.WriteStreamDone()
		}
		if err != nil {
			return err
		}
		if err := w.WriteStreamEvent(chunk); err != nil {
			return err
		}
	}
}

func (r *streamChatResp) getToolCalls() []openai.ToolCall {
	for _, resp := range r.toolCallDeltas {
		if len(resp.Choices) > 0 {
			r.accumulateToolCall(resp.Choices[0].Delta.ToolCalls)
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

	return toolCalls
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
		if v.Usage != nil && v.Usage.TotalTokens != 0 {
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

func (r *streamChatResp) updateAllowedToolIndexes(chatCtx *chatContext) {
	if r.allowedToolIndexes == nil {
		r.allowedToolIndexes = make(map[int]struct{})
	}
	for index, call := range r.toolCallsMap {
		if call.Function.Name == "" {
			continue
		}
		if isServer, ok := chatCtx.toolSources[call.Function.Name]; ok {
			if !isServer {
				r.allowedToolIndexes[index] = struct{}{}
			}
			continue
		}
		r.allowedToolIndexes[index] = struct{}{}
	}
}

func (r *streamChatResp) filterToolCalls(toolCalls []openai.ToolCall) []openai.ToolCall {
	if len(r.allowedToolIndexes) == 0 {
		return nil
	}
	filteredCalls := make([]openai.ToolCall, 0, len(toolCalls))
	for k, call := range toolCalls {
		index := k
		if call.Index != nil {
			index = *call.Index
		}
		if _, ok := r.allowedToolIndexes[index]; ok {
			filteredCalls = append(filteredCalls, call)
		}
	}
	return filteredCalls
}

func (r *streamChatResp) flushBufferedNonFinishChunks(w EventResponseWriter) error {
	if len(r.buffer) == 0 {
		return nil
	}
	remaining := r.buffer[:0]
	for _, chunk := range r.buffer {
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			remaining = append(remaining, chunk)
			continue
		}
		if err := w.WriteStreamEvent(chunk); err != nil {
			return err
		}
	}
	r.buffer = remaining
	return nil
}

func (r *streamChatResp) accumulateToolCall(delta []openai.ToolCall) {
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
		if v.ExtraContent != nil {
			item.ExtraContent = v.ExtraContent
		}
		r.toolCallsMap[index] = item
	}
}

type invokeResp struct {
	underlying       *chatResp
	includeCallStack bool
}

var _ chatResponse = (*invokeResp)(nil)

func newInvokeResp(resp openai.ChatCompletionResponse, includeCallStack bool) *invokeResp {
	return &invokeResp{
		underlying:       &chatResp{resp: resp},
		includeCallStack: includeCallStack,
	}
}

func (i *invokeResp) process(w EventResponseWriter, chatCtx *chatContext) (*processResult, error) {
	return i.underlying.processWithResponseWriteFunc(w, chatCtx, i.writeResponse)
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
