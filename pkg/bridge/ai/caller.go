package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	// CallerProviderCacheSize is the size of the caller provider cache
	CallerProviderCacheSize = 1024
	// CallerProviderCacheTTL is the time to live of the provider cache
	CallerProviderCacheTTL = time.Minute * 0
)

// CallerProvider provides the caller, which is used to interact with YoMo's stream function.
type CallerProvider interface {
	Provide(credential string) (Caller, error)
}

type callerProvider struct {
	zipperAddr  string
	exFn        ExchangeMetadataFunc
	provideFunc provideFunc
	callers     *expirable.LRU[string, Caller]
}

type provideFunc func(string, string, ExchangeMetadataFunc) (Caller, error)

// NewCallerProvider returns a new caller provider.
func NewCallerProvider(zipperAddr string, exFn ExchangeMetadataFunc) CallerProvider {
	return newCallerProvider(zipperAddr, exFn, NewCaller)
}

func newCallerProvider(zipperAddr string, exFn ExchangeMetadataFunc, provideFunc provideFunc) CallerProvider {
	p := &callerProvider{
		zipperAddr:  zipperAddr,
		exFn:        exFn,
		provideFunc: provideFunc,
		callers:     expirable.NewLRU(CallerProviderCacheSize, func(_ string, caller Caller) { caller.Close() }, CallerProviderCacheTTL),
	}

	return p
}

// Provide provides the caller according to the credential.
func (p *callerProvider) Provide(credential string) (Caller, error) {
	caller, ok := p.callers.Get(credential)
	if ok {
		return caller, nil
	}

	caller, err := p.provideFunc(credential, p.zipperAddr, p.exFn)
	if err != nil {
		return nil, err
	}
	p.callers.Add(credential, caller)

	return caller, nil
}

// Caller calls the invoke function and keeps the metadata and system prompt.
type Caller interface {
	// Call calls the invoke function.
	CallSyncer
	// SetSystemPrompt sets the system prompt of the caller.
	SetSystemPrompt(string)
	// GetSystemPrompt returns the system prompt of the caller.
	GetSystemPrompt() string
	// SetTracer sets the tracer of the caller.
	SetTracer(trace.Tracer)
	// GetTracer returns the tracer of the caller.
	GetTracer() trace.Tracer
	// Metadata returns the metadata of the caller.
	Metadata() metadata.M
	// Close closes the caller, if the caller is closed, the caller will not be reused.
	Close() error
}

type caller struct {
	CallSyncer
	source  yomo.Source
	reducer yomo.StreamFunction

	tracer       atomic.Value
	credential   string
	md           metadata.M
	systemPrompt atomic.Value
	logger       *slog.Logger
}

// NewCaller returns a new caller.
func NewCaller(credential string, zipperAddr string, exFn ExchangeMetadataFunc) (Caller, error) {
	logger := ylog.Default()

	source, reqCh, err := ChanToSource(zipperAddr, credential, logger)
	if err != nil {
		return nil, err
	}

	reducer, resCh, err := ReduceToChan(zipperAddr, credential, logger)
	if err != nil {
		return nil, err
	}

	callSyncer := NewCallSyncer(logger, reqCh, resCh, 60*time.Second)

	md, err := exFn(credential)
	if err != nil {
		return nil, err
	}

	caller := &caller{
		CallSyncer: callSyncer,
		source:     source,
		reducer:    reducer,
		md:         md,
		logger:     logger,
	}

	caller.SetSystemPrompt("")

	return caller, nil
}

// ChanToSource creates a yomo source and a channel,
// The ai.FunctionCall objects are continuously be received from the channel and be sent by the source.
func ChanToSource(zipperAddr, credential string, logger *slog.Logger) (yomo.Source, chan<- TagFunctionCall, error) {
	source := yomo.NewSource(
		"fc-source",
		zipperAddr,
		yomo.WithSourceReConnect(),
		yomo.WithCredential(credential),
	)
	err := source.Connect()
	if err != nil {
		return nil, nil, err
	}

	ch := make(chan TagFunctionCall)
	ToSource(source, logger, ch)

	return source, ch, nil
}

// ReduceToChan creates a yomo stream function to reduce the messages and returns both.
func ReduceToChan(zipperAddr, credential string, logger *slog.Logger) (yomo.StreamFunction, <-chan ReduceMessage, error) {
	reducer := yomo.NewStreamFunction(
		"ai-reducer",
		zipperAddr,
		yomo.WithSfnReConnect(),
		yomo.WithSfnCredential(credential),
		yomo.DisableOtelTrace(),
	)
	reducer.SetObserveDataTags(ai.ReducerTag)

	messages := make(chan ReduceMessage)
	ToReducer(reducer, logger, messages)

	if err := reducer.Connect(); err != nil {
		return reducer, nil, err
	}

	return reducer, messages, nil
}

// SetSystemPrompt sets the system prompt
func (c *caller) SetSystemPrompt(prompt string) {
	c.systemPrompt.Store(prompt)
}

// SetSystemPrompt gets the system prompt
func (c *caller) GetSystemPrompt() string {
	if v := c.systemPrompt.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// Metadata returns the metadata of caller.
func (c *caller) Metadata() metadata.M {
	return c.md
}

// SetTracer sets the otel tracer.
func (c *caller) SetTracer(tracer trace.Tracer) {
	c.tracer.Store(tracer)
}

// GetTracer gets the otel tracer.
func (c *caller) GetTracer() trace.Tracer {
	if v := c.tracer.Load(); v != nil {
		return v.(trace.Tracer)
	}
	return noop.NewTracerProvider().Tracer("yomo-llm-bridge")
}

// Close closes the caller.
func (c *caller) Close() error {
	_ = c.CallSyncer.Close()

	var err error
	if err = c.source.Close(); err != nil {
		c.logger.Error("callSyncer writer close", "err", err.Error())
	}

	if err = c.reducer.Close(); err != nil {
		c.logger.Error("callSyncer reducer close", "err", err.Error())
	}

	return err
}

// GetInvoke returns the invoke response
func GetInvoke(
	ctx context.Context,
	userInstruction string, baseSystemMessage string, transID string,
	includeCallStack bool,
	caller Caller, provider provider.LLMProvider,
) (*ai.InvokeResponse, error) {
	md := caller.Metadata().Clone()
	// read tools attached to the metadata
	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return &ai.InvokeResponse{}, err
	}
	// prepare tools
	tools := prepareToolCalls(tcs)

	chainMessage := ai.ChainMessage{}
	messages := prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, true)
	req := openai.ChatCompletionRequest{
		Messages: messages,
	}
	// with tools
	if len(tools) > 0 {
		req.Tools = tools
	}
	var (
		promptUsage     int
		completionUsage int
	)
	_, span := caller.GetTracer().Start(ctx, "first_call")
	chatCompletionResponse, err := provider.GetChatCompletions(ctx, req, md)
	if err != nil {
		return nil, err
	}
	span.End()
	promptUsage = chatCompletionResponse.Usage.PromptTokens
	completionUsage = chatCompletionResponse.Usage.CompletionTokens

	// convert ChatCompletionResponse to InvokeResponse
	res, err := ai.ConvertToInvokeResponse(&chatCompletionResponse, tcs)
	if err != nil {
		return nil, err
	}
	// if no tool_calls fired, just return the llm text result
	if res.FinishReason != string(openai.FinishReasonToolCalls) && len(res.ToolCalls) > 0 {
		return res, nil
	}

	// run llm function calls
	ylog.Debug(">>>> start 1st call response",
		"res_toolcalls", fmt.Sprintf("%+v", res.ToolCalls),
		"res_assistant_msgs", fmt.Sprintf("%+v", res.AssistantMessage))

	ylog.Debug(">> run function calls", "transID", transID, "res.ToolCalls", fmt.Sprintf("%+v", res.ToolCalls))

	_, span = caller.GetTracer().Start(ctx, "run_sfn")
	reqID := id.New(16)
	llmCalls, err := caller.Call(ctx, transID, reqID, res.ToolCalls)
	if err != nil {
		return nil, err
	}
	span.End()

	ylog.Debug(">>>> start 2nd call with", "calls", fmt.Sprintf("%+v", llmCalls), "preceeding_assistant_message", fmt.Sprintf("%+v", res.AssistantMessage))

	chainMessage.PreceedingAssistantMessage = res.AssistantMessage
	chainMessage.ToolMessages = transToolMessage(llmCalls)
	// do not attach toolMessage to prompt in 2nd call
	messages2 := prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, false)
	req2 := openai.ChatCompletionRequest{
		Messages: messages2,
	}
	_, span = caller.GetTracer().Start(ctx, "second_call")
	chatCompletionResponse2, err := provider.GetChatCompletions(ctx, req2, md)
	if err != nil {
		return nil, err
	}
	span.End()

	chatCompletionResponse2.Usage.PromptTokens += promptUsage
	chatCompletionResponse2.Usage.CompletionTokens += completionUsage

	res2, err := ai.ConvertToInvokeResponse(&chatCompletionResponse2, tcs)
	if err != nil {
		return nil, err
	}

	// INFO: call stack infomation
	if includeCallStack {
		res2.ToolCalls = res.ToolCalls
		res2.ToolMessages = transToolMessage(llmCalls)
	}
	ylog.Debug("<<<< complete 2nd call", "res2", fmt.Sprintf("%+v", res2))

	return res2, err
}

// GetChatCompletions accepts openai.ChatCompletionRequest and responds to http.ResponseWriter.
func GetChatCompletions(
	ctx context.Context,
	req openai.ChatCompletionRequest, transID string,
	provider provider.LLMProvider, caller Caller,
	w http.ResponseWriter,
) error {
	reqCtx, reqSpan := caller.GetTracer().Start(ctx, "completions_request")
	md := caller.Metadata().Clone()

	// 1. find all hosting tool sfn
	tagTools, err := register.ListToolCalls(md)
	if err != nil {
		return err
	}
	// 2. add those tools to request
	req = addToolsToRequest(req, tagTools)

	// 3. over write system prompt to request
	req = overWriteSystemPrompt(req, caller.GetSystemPrompt())

	var (
		promptUsage      = 0
		completionUsage  = 0
		totalUsage       = 0
		reqMessages      = req.Messages
		toolCallsMap     = make(map[int]openai.ToolCall)
		toolCalls        = []openai.ToolCall{}
		assistantMessage = openai.ChatCompletionMessage{}
	)
	// 4. request first chat for getting tools
	if req.Stream {
		_, firstCallSpan := caller.GetTracer().Start(reqCtx, "first_call_request")
		var (
			flusher        = eventFlusher(w)
			isFunctionCall = false
		)
		resStream, err := provider.GetChatCompletionsStream(reqCtx, req, md)
		if err != nil {
			return err
		}

		var (
			i             int // number of chunks
			j             int // number of tool call chunks
			firstRespSpan trace.Span
			respSpan      trace.Span
		)
		for {
			if i == 0 {
				_, firstRespSpan = caller.GetTracer().Start(reqCtx, "first_call_response_in_stream")
			}
			streamRes, err := resStream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if len(streamRes.Choices) == 0 {
				continue
			}
			if streamRes.Usage != nil {
				promptUsage = streamRes.Usage.PromptTokens
				completionUsage = streamRes.Usage.CompletionTokens
				totalUsage = streamRes.Usage.TotalTokens
			}
			if tc := streamRes.Choices[0].Delta.ToolCalls; len(tc) > 0 {
				isFunctionCall = true
				if j == 0 {
					firstCallSpan.End()
				}
				for _, t := range tc {
					// this index should be toolCalls slice's index, the index field only appares in stream response
					index := *t.Index
					item, ok := toolCallsMap[index]
					if !ok {
						toolCallsMap[index] = openai.ToolCall{
							Index:    t.Index,
							ID:       t.ID,
							Type:     t.Type,
							Function: openai.FunctionCall{},
						}
						item = toolCallsMap[index]
					}
					if t.Function.Arguments != "" {
						item.Function.Arguments += t.Function.Arguments
					}
					if t.Function.Name != "" {
						item.Function.Name = t.Function.Name
					}
					toolCallsMap[index] = item
				}
				j++
			} else if streamRes.Choices[0].FinishReason != openai.FinishReasonToolCalls {
				_ = writeStreamEvent(w, flusher, streamRes)
			}
			if i == 0 && j == 0 && !isFunctionCall {
				reqSpan.End()
				recordTTFT(ctx, caller.GetTracer())
				_, respSpan = caller.GetTracer().Start(ctx, "response_in_stream(TBT)")
			}
			i++
		}
		if !isFunctionCall {
			respSpan.End()
			return writeStreamDone(w, flusher)
		}
		firstRespSpan.End()
		toolCalls = mapToSliceTools(toolCallsMap)

		assistantMessage = openai.ChatCompletionMessage{
			ToolCalls: toolCalls,
			Role:      openai.ChatMessageRoleAssistant,
		}
		reqSpan.End()
		flusher.Flush()
	} else {
		_, firstCallSpan := caller.GetTracer().Start(reqCtx, "first_call")
		resp, err := provider.GetChatCompletions(ctx, req, md)
		if err != nil {
			return err
		}
		reqSpan.End()

		promptUsage = resp.Usage.PromptTokens
		completionUsage = resp.Usage.CompletionTokens
		totalUsage = resp.Usage.CompletionTokens

		ylog.Debug(" #1 first call", "response", fmt.Sprintf("%+v", resp))
		// it is a function call
		if (resp.Choices[0].FinishReason == openai.FinishReasonToolCalls) ||
			(len(resp.Choices[0].Message.ToolCalls) > 0) {
			toolCalls = append(toolCalls, resp.Choices[0].Message.ToolCalls...)
			assistantMessage = resp.Choices[0].Message
			firstCallSpan.End()
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return nil
		}
	}

	resCtx, resSpan := caller.GetTracer().Start(ctx, "completions_response")
	defer resSpan.End()

	_, sfnSpan := caller.GetTracer().Start(resCtx, "run_sfn")

	// 5. find sfns that hit the function call
	fnCalls := findTagTools(tagTools, toolCalls)

	// 6. run llm function calls
	reqID := id.New(16)
	llmCalls, err := caller.Call(ctx, transID, reqID, fnCalls)
	if err != nil {
		return err
	}
	sfnSpan.End()

	// 7. do the second call (the second call messages are from user input, first call resopnse and sfn calls result)
	req.Messages = append(reqMessages, assistantMessage)
	req.Messages = append(req.Messages, llmCalls...)
	req.Tools = nil // reset tools field

	ylog.Debug(" #2 second call", "request", fmt.Sprintf("%+v", req))

	if req.Stream {
		_, secondCallSpan := caller.GetTracer().Start(resCtx, "second_call_request")
		flusher := w.(http.Flusher)
		resStream, err := provider.GetChatCompletionsStream(resCtx, req, md)
		if err != nil {
			return err
		}
		secondCallSpan.End()

		var (
			i              int
			secondRespSpan trace.Span
		)
		for {
			if i == 0 {
				recordTTFT(resCtx, caller.GetTracer())
				_, secondRespSpan = caller.GetTracer().Start(resCtx, "second_call_response_in_stream(TBT)")
			}
			i++
			streamRes, err := resStream.Recv()
			if err == io.EOF {
				secondRespSpan.End()
				return writeStreamDone(w, flusher)
			}
			if err != nil {
				return err
			}
			if streamRes.Usage != nil {
				streamRes.Usage.PromptTokens += promptUsage
				streamRes.Usage.CompletionTokens += completionUsage
				streamRes.Usage.TotalTokens += totalUsage
			}
			_ = writeStreamEvent(w, flusher, streamRes)
		}
	} else {
		_, secondCallSpan := caller.GetTracer().Start(resCtx, "second_call")

		resp, err := provider.GetChatCompletions(resCtx, req, md)
		if err != nil {
			return err
		}

		resp.Usage.PromptTokens += promptUsage
		resp.Usage.CompletionTokens += completionUsage
		resp.Usage.TotalTokens += totalUsage

		secondCallSpan.End()
		ylog.Debug(" #2 second call", "response", fmt.Sprintf("%+v", resp))
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(resp)
	}
}

// ExchangeMetadataFunc is used to exchange metadata
type ExchangeMetadataFunc func(credential string) (metadata.M, error)

// DefaultExchangeMetadataFunc is the default ExchangeMetadataFunc, It returns an empty metadata.
func DefaultExchangeMetadataFunc(credential string) (metadata.M, error) {
	return metadata.M{}, nil
}

func addToolsToRequest(req openai.ChatCompletionRequest, tagTools map[uint32]openai.Tool) openai.ChatCompletionRequest {
	toolCalls := prepareToolCalls(tagTools)

	if len(toolCalls) > 0 {
		req.Tools = toolCalls
	}

	ylog.Debug(" #1 first call", "request", fmt.Sprintf("%+v", req))

	return req
}

func overWriteSystemPrompt(req openai.ChatCompletionRequest, sysPrompt string) openai.ChatCompletionRequest {
	// do nothing if system prompt is empty
	if sysPrompt == "" {
		return req
	}
	// over write system prompt
	isOverWrite := false
	for i, msg := range req.Messages {
		if msg.Role != "system" {
			continue
		}
		req.Messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: sysPrompt,
		}
		isOverWrite = true
	}
	// append system prompt
	if !isOverWrite {
		req.Messages = append(req.Messages, openai.ChatCompletionMessage{
			Role:    "system",
			Content: sysPrompt,
		})
	}

	ylog.Debug(" #1 first call after overwrite", "request", fmt.Sprintf("%+v", req))

	return req
}

func findTagTools(tagTools map[uint32]openai.Tool, toolCalls []openai.ToolCall) map[uint32][]*openai.ToolCall {
	fnCalls := make(map[uint32][]*openai.ToolCall)
	// functions may be more than one
	for _, call := range toolCalls {
		for tag, tc := range tagTools {
			if tc.Function.Name == call.Function.Name && tc.Type == call.Type {
				currentCall := call
				fnCalls[tag] = append(fnCalls[tag], &currentCall)
			}
		}
	}
	return fnCalls
}

func writeStreamEvent(w http.ResponseWriter, flusher http.Flusher, streamRes openai.ChatCompletionStreamResponse) error {
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(streamRes); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	flusher.Flush()

	return nil
}

func writeStreamDone(w http.ResponseWriter, flusher http.Flusher) error {
	_, err := io.WriteString(w, "data: [DONE]")
	flusher.Flush()

	return err
}

func prepareMessages(baseSystemMessage string, userInstruction string, chainMessage ai.ChainMessage, tools []openai.Tool, withTool bool) []openai.ChatCompletionMessage {
	systemInstructions := []string{"## Instructions\n"}

	// only append if there are tool calls
	if withTool {
		for _, t := range tools {
			systemInstructions = append(systemInstructions, "- ")
			systemInstructions = append(systemInstructions, t.Function.Description)
			systemInstructions = append(systemInstructions, "\n")
		}
		systemInstructions = append(systemInstructions, "\n")
	}

	SystemPrompt := fmt.Sprintf("%s\n\n%s", baseSystemMessage, strings.Join(systemInstructions, ""))

	messages := []openai.ChatCompletionMessage{}

	// 1. system message
	messages = append(messages, openai.ChatCompletionMessage{Role: "system", Content: SystemPrompt})

	// 2. previous tool calls
	// Ref: Tool Message Object in Messsages
	// https://platform.openai.com/docs/guides/function-calling
	// https://platform.openai.com/docs/api-reference/chat/create#chat-create-messages

	if chainMessage.PreceedingAssistantMessage != nil {
		// 2.1 assistant message
		// try convert type of chainMessage.PreceedingAssistantMessage to type ChatCompletionMessage
		assistantMessage, ok := chainMessage.PreceedingAssistantMessage.(openai.ChatCompletionMessage)
		if ok {
			ylog.Debug("======== add assistantMessage", "am", fmt.Sprintf("%+v", assistantMessage))
			messages = append(messages, assistantMessage)
		}

		// 2.2 tool message
		for _, tool := range chainMessage.ToolMessages {
			tm := openai.ChatCompletionMessage{
				Role:       "tool",
				Content:    tool.Content,
				ToolCallID: tool.ToolCallID,
			}
			ylog.Debug("======== add toolMessage", "tm", fmt.Sprintf("%+v", tm))
			messages = append(messages, tm)
		}
	}

	// 3. user instruction
	messages = append(messages, openai.ChatCompletionMessage{Role: "user", Content: userInstruction})

	return messages
}

func mapToSliceTools(m map[int]openai.ToolCall) []openai.ToolCall {
	arr := make([]openai.ToolCall, len(m))
	for k, v := range m {
		arr[k] = v
	}
	return arr
}

func eventFlusher(w http.ResponseWriter) http.Flusher {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, must-revalidate")
	h.Set("x-content-type-options", "nosniff")
	flusher := w.(http.Flusher)
	return flusher
}

func prepareToolCalls(tcs map[uint32]openai.Tool) []openai.Tool {
	// prepare tools
	toolCalls := make([]openai.Tool, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = tc
		idx++
	}
	return toolCalls
}

func transToolMessage(msgs []openai.ChatCompletionMessage) []ai.ToolMessage {
	toolMessages := make([]ai.ToolMessage, len(msgs))
	for i, msg := range msgs {
		toolMessages[i] = ai.ToolMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
	}
	return toolMessages
}

func recordTTFT(ctx context.Context, tracer trace.Tracer) {
	_, span := tracer.Start(ctx, "TTFT")
	span.End()
	time.Sleep(time.Millisecond)
}
