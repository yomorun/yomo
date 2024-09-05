package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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

// Service is the  service layer for llm bridge server.
// service is responsible for handling the logic from handler layer.
type Service struct {
	zipperAddr    string
	provider      provider.LLMProvider
	newCallerFunc newCallerFunc
	callers       *expirable.LRU[string, *Caller]
	option        *ServiceOptions
	logger        *slog.Logger
}

// ServiceOptions is the option for creating service
type ServiceOptions struct {
	// Logger is the logger for the service
	Logger *slog.Logger
	// Tracer is the tracer for the service
	Tracer trace.Tracer
	// CredentialFunc is the function for getting the credential from the request
	CredentialFunc func(r *http.Request) (string, error)
	// CallerCacheSize is the size of the caller's cache
	CallerCacheSize int
	// CallerCacheTTL is the time to live of the callers cache
	CallerCacheTTL time.Duration
	// CallerCallTimeout is the timeout for awaiting the function response.
	CallerCallTimeout time.Duration
	// SourceBuilder should builds an unconnected source.
	SourceBuilder func(zipperAddr, credential string) yomo.Source
	// ReducerBuilder should builds an unconnected reducer.
	ReducerBuilder func(zipperAddr, credential string) yomo.StreamFunction
	// MetadataExchanger exchanges metadata from the credential.
	MetadataExchanger func(credential string) (metadata.M, error)
}

// NewService creates a new service for handling the logic from handler layer.
func NewService(zipperAddr string, provider provider.LLMProvider, opt *ServiceOptions) *Service {
	return newService(zipperAddr, provider, NewCaller, opt)
}

func initOption(opt *ServiceOptions) *ServiceOptions {
	if opt == nil {
		opt = &ServiceOptions{}
	}
	if opt.Tracer == nil {
		opt.Tracer = noop.NewTracerProvider().Tracer("yomo-ai-bridge")
	}
	if opt.Logger == nil {
		opt.Logger = ylog.Default()
	}
	if opt.CredentialFunc == nil {
		opt.CredentialFunc = func(_ *http.Request) (string, error) { return "", nil }
	}
	if opt.CallerCacheSize == 0 {
		opt.CallerCacheSize = 1
	}
	if opt.CallerCallTimeout == 0 {
		opt.CallerCallTimeout = 60 * time.Second
	}
	if opt.SourceBuilder == nil {
		opt.SourceBuilder = func(zipperAddr, credential string) yomo.Source {
			return yomo.NewSource(
				"fc-source",
				zipperAddr,
				yomo.WithSourceReConnect(), yomo.WithCredential(credential))
		}
	}
	if opt.ReducerBuilder == nil {
		opt.ReducerBuilder = func(zipperAddr, credential string) yomo.StreamFunction {
			return yomo.NewStreamFunction(
				"fc-reducer",
				zipperAddr,
				yomo.WithSfnReConnect(), yomo.WithSfnCredential(credential), yomo.DisableOtelTrace())
		}
	}
	if opt.MetadataExchanger == nil {
		opt.MetadataExchanger = func(credential string) (metadata.M, error) {
			return metadata.New(), nil
		}
	}

	return opt
}

func newService(zipperAddr string, provider provider.LLMProvider, ncf newCallerFunc, opt *ServiceOptions) *Service {
	var onEvict = func(_ string, caller *Caller) {
		caller.Close()
	}

	opt = initOption(opt)

	service := &Service{
		zipperAddr:    zipperAddr,
		provider:      provider,
		newCallerFunc: ncf,
		callers:       expirable.NewLRU(opt.CallerCacheSize, onEvict, opt.CallerCacheTTL),
		option:        opt,
		logger:        opt.Logger,
	}

	return service
}

type newCallerFunc func(yomo.Source, yomo.StreamFunction, metadata.M, time.Duration) (*Caller, error)

// LoadOrCreateCaller loads or creates the caller according to the http request.
func (srv *Service) LoadOrCreateCaller(r *http.Request) (*Caller, error) {
	credential, err := srv.option.CredentialFunc(r)
	if err != nil {
		return nil, err
	}
	return srv.loadOrCreateCaller(credential)
}

// GetInvoke returns the invoke response
func (srv *Service) GetInvoke(ctx context.Context, userInstruction, baseSystemMessage, transID string, caller *Caller, includeCallStack bool) (*ai.InvokeResponse, error) {
	md := caller.Metadata().Clone()
	// read tools attached to the metadata
	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return &ai.InvokeResponse{}, err
	}
	// prepare tools
	tools := prepareToolCalls(tcs)

	chainMessage := ai.ChainMessage{}
	messages := srv.prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, true)
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
	_, span := srv.option.Tracer.Start(ctx, "first_call")
	chatCompletionResponse, err := srv.provider.GetChatCompletions(ctx, req, md)
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
	if res.FinishReason != string(openai.FinishReasonToolCalls) {
		return res, nil
	}

	// run llm function calls
	srv.logger.Debug(">>>> start 1st call response",
		"res_toolcalls", fmt.Sprintf("%+v", res.ToolCalls),
		"res_assistant_msgs", fmt.Sprintf("%+v", res.AssistantMessage))

	srv.logger.Debug(">> run function calls", "transID", transID, "res.ToolCalls", fmt.Sprintf("%+v", res.ToolCalls))

	_, span = srv.option.Tracer.Start(ctx, "run_sfn")
	reqID := id.New(16)
	llmCalls, err := caller.Call(ctx, transID, reqID, res.ToolCalls)
	if err != nil {
		return nil, err
	}
	span.End()

	srv.logger.Debug(">>>> start 2nd call with", "calls", fmt.Sprintf("%+v", llmCalls), "preceeding_assistant_message", fmt.Sprintf("%+v", res.AssistantMessage))

	chainMessage.PreceedingAssistantMessage = res.AssistantMessage
	chainMessage.ToolMessages = transToolMessage(llmCalls)
	// do not attach toolMessage to prompt in 2nd call
	messages2 := srv.prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, false)
	req2 := openai.ChatCompletionRequest{
		Messages: messages2,
	}
	_, span = srv.option.Tracer.Start(ctx, "second_call")
	chatCompletionResponse2, err := srv.provider.GetChatCompletions(ctx, req2, md)
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
	srv.logger.Debug("<<<< complete 2nd call", "res2", fmt.Sprintf("%+v", res2))

	return res2, err
}

// GetChatCompletions accepts openai.ChatCompletionRequest and responds to http.ResponseWriter.
func (srv *Service) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, transID string, caller *Caller, w http.ResponseWriter) error {
	reqCtx, reqSpan := srv.option.Tracer.Start(ctx, "completions_request")
	md := caller.Metadata().Clone()

	// 1. find all hosting tool sfn
	tagTools, err := register.ListToolCalls(md)
	if err != nil {
		return err
	}
	// 2. add those tools to request
	req = srv.addToolsToRequest(req, tagTools)

	// 3. over write system prompt to request
	req = srv.overWriteSystemPrompt(req, caller.GetSystemPrompt())

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
		_, firstCallSpan := srv.option.Tracer.Start(reqCtx, "first_call_request")

		resStream, err := srv.provider.GetChatCompletionsStream(reqCtx, req, md)
		if err != nil {
			return err
		}
		var (
			flusher        = eventFlusher(w)
			isFunctionCall = false
		)
		var (
			i             int // number of chunks
			j             int // number of tool call chunks
			firstRespSpan trace.Span
			respSpan      trace.Span
		)
		for {
			if i == 0 {
				_, firstRespSpan = srv.option.Tracer.Start(reqCtx, "first_call_response_in_stream")
			}
			streamRes, err := resStream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if streamRes.Usage != nil {
				promptUsage = streamRes.Usage.PromptTokens
				completionUsage = streamRes.Usage.CompletionTokens
				totalUsage = streamRes.Usage.TotalTokens
			}
			if len(streamRes.Choices) == 0 {
				continue
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
				recordTTFT(ctx, srv.option.Tracer)
				_, respSpan = srv.option.Tracer.Start(ctx, "response_in_stream(TBT)")
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
		_, firstCallSpan := srv.option.Tracer.Start(reqCtx, "first_call")
		resp, err := srv.provider.GetChatCompletions(ctx, req, md)
		if err != nil {
			return err
		}
		reqSpan.End()

		promptUsage = resp.Usage.PromptTokens
		completionUsage = resp.Usage.CompletionTokens
		totalUsage = resp.Usage.CompletionTokens

		srv.logger.Debug(" #1 first call", "response", fmt.Sprintf("%+v", resp))
		// it is a function call
		if resp.Choices[0].FinishReason == openai.FinishReasonToolCalls {
			toolCalls = append(toolCalls, resp.Choices[0].Message.ToolCalls...)
			assistantMessage = resp.Choices[0].Message
			firstCallSpan.End()
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return nil
		}
	}

	resCtx, resSpan := srv.option.Tracer.Start(ctx, "completions_response")
	defer resSpan.End()

	_, sfnSpan := srv.option.Tracer.Start(resCtx, "run_sfn")

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

	srv.logger.Debug(" #2 second call", "request", fmt.Sprintf("%+v", req))

	if req.Stream {
		_, secondCallSpan := srv.option.Tracer.Start(resCtx, "second_call_request")
		flusher := w.(http.Flusher)
		resStream, err := srv.provider.GetChatCompletionsStream(resCtx, req, md)
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
				recordTTFT(resCtx, srv.option.Tracer)
				_, secondRespSpan = srv.option.Tracer.Start(resCtx, "second_call_response_in_stream(TBT)")
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
		_, secondCallSpan := srv.option.Tracer.Start(resCtx, "second_call")

		resp, err := srv.provider.GetChatCompletions(resCtx, req, md)
		if err != nil {
			return err
		}

		resp.Usage.PromptTokens += promptUsage
		resp.Usage.CompletionTokens += completionUsage
		resp.Usage.TotalTokens += totalUsage

		secondCallSpan.End()
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(resp)
	}
}

func (srv *Service) loadOrCreateCaller(credential string) (*Caller, error) {
	caller, ok := srv.callers.Get(credential)
	if ok {
		return caller, nil
	}
	md, err := srv.option.MetadataExchanger(credential)
	if err != nil {
		return nil, err
	}
	caller, err = srv.newCallerFunc(
		srv.option.SourceBuilder(srv.zipperAddr, credential),
		srv.option.ReducerBuilder(srv.zipperAddr, credential),
		md,
		srv.option.CallerCallTimeout,
	)
	if err != nil {
		return nil, err
	}

	srv.callers.Add(credential, caller)

	return caller, nil
}

func (srv *Service) addToolsToRequest(req openai.ChatCompletionRequest, tagTools map[uint32]openai.Tool) openai.ChatCompletionRequest {
	toolCalls := prepareToolCalls(tagTools)

	if len(toolCalls) > 0 {
		req.Tools = toolCalls
	}

	srv.logger.Debug(" #1 first call", "request", fmt.Sprintf("%+v", req))

	return req
}

func (srv *Service) overWriteSystemPrompt(req openai.ChatCompletionRequest, sysPrompt string) openai.ChatCompletionRequest {
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

	srv.logger.Debug(" #1 first call after overwrite", "request", fmt.Sprintf("%+v", req))

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

func (srv *Service) prepareMessages(baseSystemMessage string, userInstruction string, chainMessage ai.ChainMessage, tools []openai.Tool, withTool bool) []openai.ChatCompletionMessage {
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
			srv.logger.Debug("======== add assistantMessage", "am", fmt.Sprintf("%+v", assistantMessage))
			messages = append(messages, assistantMessage)
		}

		// 2.2 tool message
		for _, tool := range chainMessage.ToolMessages {
			tm := openai.ChatCompletionMessage{
				Role:       "tool",
				Content:    tool.Content,
				ToolCallID: tool.ToolCallID,
			}
			srv.logger.Debug("======== add toolMessage", "tm", fmt.Sprintf("%+v", tm))
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
	time.Sleep(time.Millisecond)
	span.End()
	time.Sleep(time.Millisecond)
}
