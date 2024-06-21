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
)

var (
	// CallerProviderCacheSize is the size of the caller provider cache
	CallerProviderCacheSize = 1024
	// CallerProviderCacheTTL is the time to live of the provider cache
	CallerProviderCacheTTL = time.Minute * 0
)

// CallerProvider provides the caller, which is used to interact with YoMo's stream function.
type CallerProvider struct {
	lp          provider.LLMProvider
	zipperAddr  string
	exFn        ExchangeMetadataFunc
	provideFunc provideFunc
	callers     *expirable.LRU[string, *Caller]
}

type provideFunc func(string, string, provider.LLMProvider, ExchangeMetadataFunc) (*Caller, error)

// NewCallerProvider returns a new caller provider.
func NewCallerProvider(zipperAddr string, lp provider.LLMProvider, exFn ExchangeMetadataFunc) *CallerProvider {
	return newCallerProvider(zipperAddr, lp, exFn, NewCaller)
}

func newCallerProvider(zipperAddr string, lp provider.LLMProvider, exFn ExchangeMetadataFunc, provideFunc provideFunc) *CallerProvider {
	p := &CallerProvider{
		zipperAddr:  zipperAddr,
		lp:          lp,
		exFn:        exFn,
		provideFunc: provideFunc,
		callers:     expirable.NewLRU(CallerProviderCacheSize, func(_ string, caller *Caller) { caller.Close() }, CallerProviderCacheTTL),
	}

	return p
}

// Provide provides the caller according to the credential.
func (p *CallerProvider) Provide(credential string) (*Caller, error) {
	caller, ok := p.callers.Get(credential)
	if ok {
		return caller, nil
	}

	caller, err := p.provideFunc(credential, p.zipperAddr, p.lp, p.exFn)
	if err != nil {
		return nil, err
	}
	p.callers.Add(credential, caller)

	return caller, nil
}

// Caller calls the invoke function and the chat completion function.
type Caller struct {
	CallSyncer

	credential   string
	md           metadata.M
	systemPrompt atomic.Value
	provider     provider.LLMProvider
}

// NewCaller returns a new caller.
func NewCaller(credential string, zipperAddr string, provider provider.LLMProvider, exFn ExchangeMetadataFunc) (*Caller, error) {
	source := yomo.NewSource(
		"fc-source",
		zipperAddr,
		yomo.WithSourceReConnect(),
		yomo.WithCredential(credential),
	)
	err := source.Connect()
	if err != nil {
		return nil, err
	}

	reducer := yomo.NewStreamFunction(
		"ai-reducer",
		zipperAddr,
		yomo.WithSfnReConnect(),
		yomo.WithSfnCredential(credential),
	)
	reducer.SetObserveDataTags(ai.ReducerTag)

	// this line must before `Connect()`, because it should sets hander before connect.
	callSyncer := NewCallSyncer(slog.Default(), source, reducer, 60*time.Second)

	if err := reducer.Connect(); err != nil {
		return nil, err
	}

	md, err := exFn(credential)
	if err != nil {
		return nil, err
	}

	caller := &Caller{
		CallSyncer: callSyncer,
		credential: credential,
		md:         md,
		provider:   provider,
	}

	caller.SetSystemPrompt("")

	return caller, nil
}

// SetSystemPrompt sets the system prompt
func (c *Caller) SetSystemPrompt(prompt string) {
	c.systemPrompt.Store(prompt)
}

// Metadata returns the metadata of caller.
func (c *Caller) Metadata() metadata.M {
	return c.md
}

// GetInvoke returns the invoke response
func (c *Caller) GetInvoke(ctx context.Context, userInstruction string, baseSystemMessage string, transID string, includeCallStack bool) (*ai.InvokeResponse, error) {
	// read tools attached to the metadata
	tcs, err := register.ListToolCalls(c.md)
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
	chatCompletionResponse, err := c.provider.GetChatCompletions(ctx, req)
	if err != nil {
		return nil, err
	}

	promptUsage = chatCompletionResponse.Usage.PromptTokens
	completionUsage = chatCompletionResponse.Usage.CompletionTokens

	// convert ChatCompletionResponse to InvokeResponse
	res, err := ai.ConvertToInvokeResponse(&chatCompletionResponse, tcs)
	if err != nil {
		return nil, err
	}
	// if no tool_calls fired, just return the llm text result
	if !(res.FinishReason == "tool_calls" || res.FinishReason == "gemini_tool_calls") {
		return res, nil
	}

	// run llm function calls
	ylog.Debug(">>>> start 1st call response",
		"res_toolcalls", fmt.Sprintf("%+v", res.ToolCalls),
		"res_assistant_msgs", fmt.Sprintf("%+v", res.AssistantMessage))

	ylog.Debug(">> run function calls", "transID", transID, "res.ToolCalls", fmt.Sprintf("%+v", res.ToolCalls))

	reqID := id.New(16)
	llmCalls, err := c.Call(ctx, transID, reqID, res.ToolCalls)
	if err != nil {
		return nil, err
	}

	ylog.Debug(">>>> start 2nd call with", "calls", fmt.Sprintf("%+v", llmCalls), "preceeding_assistant_message", fmt.Sprintf("%+v", res.AssistantMessage))

	chainMessage.PreceedingAssistantMessage = res.AssistantMessage
	chainMessage.ToolMessages = transToolMessage(llmCalls)
	// do not attach toolMessage to prompt in 2nd call
	messages2 := prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, false)
	req2 := openai.ChatCompletionRequest{
		Messages: messages2,
	}
	chatCompletionResponse2, err := c.provider.GetChatCompletions(ctx, req2)
	if err != nil {
		return nil, err
	}

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
func (c *Caller) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, transID string, w http.ResponseWriter) error {
	// 1. find all hosting tool sfn
	tagTools, err := register.ListToolCalls(c.md)
	if err != nil {
		return err
	}
	// 2. add those tools to request
	req = addToolsToRequest(req, tagTools)

	// 3. over write system prompt to request
	req = overWriteSystemPrompt(req, c.systemPrompt.Load().(string))

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
		var (
			flusher        = eventFlusher(w)
			isFunctionCall = false
		)
		resStream, err := c.provider.GetChatCompletionsStream(ctx, req)
		if err != nil {
			return err
		}
		for {
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
				isFunctionCall = true
			} else if streamRes.Choices[0].FinishReason != openai.FinishReasonToolCalls {
				_ = writeStreamEvent(w, flusher, streamRes)
			}
		}
		if !isFunctionCall {
			return writeStreamDone(w, flusher)
		}
		toolCalls = mapToSliceTools(toolCallsMap)

		assistantMessage = openai.ChatCompletionMessage{
			ToolCalls: toolCalls,
			Role:      openai.ChatMessageRoleAssistant,
		}
		flusher.Flush()
	} else {
		resp, err := c.provider.GetChatCompletions(ctx, req)
		if err != nil {
			return err
		}
		promptUsage = resp.Usage.PromptTokens
		completionUsage = resp.Usage.CompletionTokens
		totalUsage = resp.Usage.CompletionTokens

		ylog.Debug(" #1 first call", "response", fmt.Sprintf("%+v", resp))
		// it is a function call
		if resp.Choices[0].FinishReason == openai.FinishReasonToolCalls {
			toolCalls = append(toolCalls, resp.Choices[0].Message.ToolCalls...)
			assistantMessage = resp.Choices[0].Message
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return nil
		}
	}

	// 5. find sfns that hit the function call
	fnCalls := findTagTools(tagTools, toolCalls)

	// 6. run llm function calls
	reqID := id.New(16)
	llmCalls, err := c.Call(ctx, transID, reqID, fnCalls)
	if err != nil {
		return err
	}

	// 7. do the second call (the second call messages are from user input, first call resopnse and sfn calls result)
	req.Messages = append(reqMessages, assistantMessage)
	req.Messages = append(req.Messages, llmCalls...)
	req.Tools = nil // reset tools field

	ylog.Debug(" #2 second call", "request", fmt.Sprintf("%+v", req))

	if req.Stream {
		flusher := w.(http.Flusher)
		resStream, err := c.provider.GetChatCompletionsStream(ctx, req)
		if err != nil {
			return err
		}
		for {
			streamRes, err := resStream.Recv()
			if err == io.EOF {
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
		resp, err := c.provider.GetChatCompletions(ctx, req)
		if err != nil {
			return err
		}

		resp.Usage.PromptTokens += promptUsage
		resp.Usage.CompletionTokens += completionUsage
		resp.Usage.TotalTokens += totalUsage

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
