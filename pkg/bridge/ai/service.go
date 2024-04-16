package ai

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/serverless"
)

var (
	// ServiceCacheSize is the size of the service cache
	ServiceCacheSize = 1024
	// ServiceCacheTTL is the time to live of the service cache
	ServiceCacheTTL = time.Minute * 0 // 30
	// services is the cache of Service
	services *expirable.LRU[string, *Service]
)

// Service is used to invoke LLM Provider to get the functions to be executed,
// then, use source to send arguments which returned by llm provider to target
// function. Finally, use reducer to aggregate all the results, and write the
// result by the http.ResponseWriter.
type Service struct {
	credential   string
	zipperAddr   string
	md           metadata.M
	source       yomo.Source
	reducer      yomo.StreamFunction
	sfnCallCache map[string]*sfnAsyncCall
	muCallCache  sync.Mutex
	LLMProvider
}

// LoadOrCreateService loads or creates a new AI service, if the service is already created, it will return the existing one
func LoadOrCreateService(credential string, zipperAddr string, aiProvider LLMProvider, exFn ExchangeMetadataFunc) (*Service, error) {
	s, ok := services.Get(credential)
	if ok {
		return s, nil
	}
	s, err := newService(credential, zipperAddr, aiProvider, exFn)
	if err != nil {
		return nil, err
	}
	services.Add(credential, s)
	return s, nil
}

// ExchangeMetadataFunc is used to exchange metadata
type ExchangeMetadataFunc func(credential string) (metadata.M, error)

// DefaultExchangeMetadataFunc is the default ExchangeMetadataFunc, It returns an empty metadata.
func DefaultExchangeMetadataFunc(credential string) (metadata.M, error) {
	return metadata.M{}, nil
}

func newService(credential string, zipperAddr string, aiProvider LLMProvider, exFn ExchangeMetadataFunc) (*Service, error) {
	s := &Service{
		credential: credential,
		zipperAddr: zipperAddr,
		// cache:        make(map[string]*CacheItem),
		LLMProvider:  aiProvider,
		sfnCallCache: make(map[string]*sfnAsyncCall),
	}
	// metadata
	if exFn == nil {
		s.md = metadata.M{}
	} else {
		md, err := exFn(credential)
		if err != nil {
			ylog.Error("exchange metadata failed", "err", err)
			return nil, err
		}
		s.md = md
	}

	// source
	source, err := s.createSource()
	if err != nil {
		ylog.Error("create fc-service source failed", "err", err)
		return nil, err
	}
	s.source = source
	// reducer
	reducer, err := s.createReducer()
	if err != nil {
		ylog.Error("create fc-service reducer failed", "err", err)
		return nil, err
	}
	s.reducer = reducer
	return s, nil
}

// Release releases the resources
func (s *Service) Release() {
	ylog.Debug("release AI service", "credential", s.credential)
	if s.source != nil {
		s.source.Close()
	}
	if s.reducer != nil {
		s.reducer.Close()
	}
}

func (s *Service) createSource() (yomo.Source, error) {
	ylog.Debug("create fc-service source", "zipperAddr", s.zipperAddr, "credential", s.credential)
	source := yomo.NewSource(
		"fc-source",
		s.zipperAddr,
		yomo.WithSourceReConnect(),
		yomo.WithCredential(s.credential),
	)
	// create ai source
	err := source.Connect()
	if err != nil {
		return nil, err
	}
	return source, nil
}

// createReducer creates the reducer-sfn. reducer-sfn used to aggregate all the llm-sfn execute results.
func (s *Service) createReducer() (yomo.StreamFunction, error) {
	sfn := yomo.NewStreamFunction(
		"ai-reducer",
		s.zipperAddr,
		yomo.WithSfnReConnect(),
		yomo.WithSfnCredential(s.credential),
	)
	sfn.SetObserveDataTags(ai.ReducerTag)
	sfn.SetHandler(func(ctx serverless.Context) {
		buf := ctx.Data()
		ylog.Debug("[sfn-reducer]", "tag", ai.ReducerTag, "data", string(buf))
		invoke, err := ai.ParseFunctionCallContext(ctx)
		if err != nil {
			ylog.Error("[sfn-reducer] parse function calling invoke", "err", err.Error())
			return
		}

		reqID := invoke.ReqID

		// write parallel function calling results to cache, after all the results are written, the reducer will be done
		s.muCallCache.Lock()
		c, ok := s.sfnCallCache[reqID]
		s.muCallCache.Unlock()
		if !ok {
			ylog.Error("[sfn-reducer] req_id not found", "req_id", reqID)
			return
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		// need lock c.val as multiple handler channel will write to it
		c.val[invoke.ToolCallID] = ai.ToolMessage{
			Content:    invoke.Result,
			ToolCallId: invoke.ToolCallID,
		}
		ylog.Debug("[sfn-reducer] generate", "ToolMessage", fmt.Sprintf("%+v", c.val))

		c.wg.Done()
	})

	err := sfn.Connect()
	if err != nil {
		return nil, err
	}
	return sfn, nil
}

// GetOverview returns the overview of the AI functions, key is the tag, value is the function definition
func (s *Service) GetOverview() (*ai.OverviewResponse, error) {
	tcs, err := register.ListToolCalls(s.md)
	if err != nil {
		return &ai.OverviewResponse{}, err
	}

	functions := make(map[uint32]*openai.FunctionDefinition)
	for tag, tc := range tcs {
		functions[tag] = tc.Function
	}

	return &ai.OverviewResponse{Functions: functions}, nil
}

// GetInvoke returns the invoke response
func (s *Service) GetInvoke(userInstruction string, baseSystemMessage string, reqID string, includeCallStack bool) (*ai.InvokeResponse, error) {
	// read tools attached to the metadata
	tcs, err := register.ListToolCalls(s.md)
	if err != nil {
		return &ai.InvokeResponse{}, err
	}
	// prepare tools
	tools, err := prepareToolCalls(tcs)
	if err != nil {
		return nil, err
	}
	chainMessage := ai.ChainMessage{}
	messages := prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, true)
	req := openai.ChatCompletionRequest{
		Messages: messages,
	}
	// with tools
	if len(tools) > 0 {
		req.Tools = tools
	}
	chatCompletionResponse, err := s.LLMProvider.GetChatCompletions(req)
	if err != nil {
		return nil, err
	}
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

	ylog.Debug(">> run function calls", "reqID", reqID, "res.ToolCalls", fmt.Sprintf("%+v", res.ToolCalls))
	llmCalls, err := s.runFunctionCalls(res.ToolCalls, reqID)
	if err != nil {
		return nil, err
	}

	ylog.Debug(">>>> start 2nd call with", "calls", fmt.Sprintf("%+v", llmCalls), "preceeding_assistant_message", fmt.Sprintf("%+v", res.AssistantMessage))
	chainMessage.PreceedingAssistantMessage = res.AssistantMessage
	chainMessage.ToolMessages = llmCalls
	// do not attach toolMessage to prompt in 2nd call
	messages2 := prepareMessages(baseSystemMessage, userInstruction, chainMessage, tools, false)
	req2 := openai.ChatCompletionRequest{
		Messages: messages2,
	}
	chatCompletionResponse2, err := s.LLMProvider.GetChatCompletions(req2)
	if err != nil {
		return nil, err
	}
	res2, err := ai.ConvertToInvokeResponse(&chatCompletionResponse2, tcs)
	if err != nil {
		return nil, err
	}

	// INFO: call stack infomation
	if includeCallStack {
		res2.ToolCalls = res.ToolCalls
		res2.ToolMessages = llmCalls
	}
	ylog.Debug("<<<< complete 2nd call", "res2", fmt.Sprintf("%+v", res2))

	return res2, err
}

// GetChatCompletions returns the llm api response
func (s *Service) GetChatCompletions(req openai.ChatCompletionRequest, reqID string, includeCallStack bool) (openai.ChatCompletionResponse, error) {
	// TODO: support streaming response.
	if req.Stream {
		return openai.ChatCompletionResponse{}, errors.New("streaming response not supported")
	}

	// read tools attached to the metadata
	tcs, err := register.ListToolCalls(s.md)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	// prepare tools
	toolCalls, err := prepareToolCalls(tcs)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	rawMessages := req.Messages
	if len(toolCalls) > 0 {
		req.Tools = toolCalls
	}
	ylog.Debug(" #1 first call", "request", fmt.Sprintf("%+v", req))
	// #1 first call
	res, err := s.LLMProvider.GetChatCompletions(req)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	ylog.Debug(" #1 first call", "response", fmt.Sprintf("%+v", res))
	finishReason := res.Choices[0].FinishReason
	if strings.ToLower(string(finishReason)) == "stop" {
		return res, nil
	}
	var assistantMessage openai.ChatCompletionMessage
	// if no tool_calls fired, just return the llm text result
	if finishReason == "tool_calls" || finishReason == "gemini_tool_calls" {
		assistantMessage = res.Choices[0].Message
	} else {
		return res, nil
	}

	resCalls := res.Choices[0].Message.ToolCalls
	// ready call sfns
	fnCalls := make(map[uint32][]*openai.ToolCall)
	// functions may be more than one
	for _, call := range resCalls {
		for tag, tc := range tcs {
			if (tc.Function.Name == call.Function.Name && tc.Type == call.Type) || finishReason == "gemini_tool_calls" {
				currentCall := call
				fnCalls[tag] = append(fnCalls[tag], &currentCall)
			}
		}
	}
	// run llm function calls
	llmCalls, err := s.runFunctionCalls(fnCalls, reqID)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	// #2 second call
	req.Messages = append(rawMessages, assistantMessage)
	// tool message
	for _, tool := range llmCalls {
		tm := openai.ChatCompletionMessage{
			Role:       "tool",
			Content:    tool.Content,
			ToolCallID: tool.ToolCallId,
		}
		req.Messages = append(req.Messages, tm)
	}
	// reset tools
	req.Tools = nil

	ylog.Debug(" #2 second call", "request", fmt.Sprintf("%+v", req))
	res2, err := s.LLMProvider.GetChatCompletions(req)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	ylog.Debug(" #2 second call", "request", fmt.Sprintf("%+v", req))
	return res2, err
}

// run llm-sfn function calls
func (s *Service) runFunctionCalls(fns map[uint32][]*openai.ToolCall, reqID string) ([]ai.ToolMessage, error) {
	asyncCall := &sfnAsyncCall{
		wg:  &sync.WaitGroup{},
		val: make(map[string]ai.ToolMessage),
	}
	s.muCallCache.Lock()
	s.sfnCallCache[reqID] = asyncCall
	s.muCallCache.Unlock()

	for tag, tcs := range fns {
		ylog.Debug("+++invoke toolCalls", "tag", tag, "len(toolCalls)", len(tcs), "reqID", reqID)
		for _, fn := range tcs {
			err := s.fireLlmSfn(tag, fn, reqID)
			if err != nil {
				ylog.Error("send data to zipper", "err", err.Error())
				continue
			}
			// wait for this request to be done
			asyncCall.wg.Add(1 * register.SfnFactor(tag, s.md))
		}
	}

	// wait for reducer to finish, the aggregation results
	asyncCall.wg.Wait()

	arr := make([]ai.ToolMessage, 0)

	asyncCall.mu.RLock()
	for _, call := range asyncCall.val {
		ylog.Debug("---invoke done", "id", call.ToolCallId, "content", call.Content)
		call.Role = "tool"
		arr = append(arr, call)
	}
	asyncCall.mu.RUnlock()

	return arr, nil
}

// fireLlmSfn fires the llm-sfn function call by s.source.Write()
func (s *Service) fireLlmSfn(tag uint32, fn *openai.ToolCall, reqID string) error {
	ylog.Info(
		"+invoke func",
		"tag", tag,
		"toolCallID", fn.ID,
		"function", fn.Function.Name,
		"arguments", fn.Function.Arguments,
		"reqID", reqID)
	data := &ai.FunctionCall{
		ReqID:        reqID,
		ToolCallID:   fn.ID,
		Arguments:    fn.Function.Arguments,
		FunctionName: fn.Function.Name,
	}
	buf, err := data.Bytes()
	if err != nil {
		ylog.Error("marshal data", "err", err.Error())
	}
	return s.source.Write(tag, buf)
}

// Write writes the data to zipper
func (s *Service) Write(tag uint32, data []byte) error {
	return s.source.Write(tag, data)
}

func init() {
	onEvicted := func(_ string, v *Service) {
		v.Release()
	}
	services = expirable.NewLRU(ServiceCacheSize, onEvicted, ServiceCacheTTL)
}

type sfnAsyncCall struct {
	wg  *sync.WaitGroup
	mu  sync.RWMutex
	val map[string]ai.ToolMessage
}

func prepareToolCalls(tcs map[uint32]openai.Tool) ([]openai.Tool, error) {
	// prepare tools
	toolCalls := make([]openai.Tool, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = tc
		idx++
	}
	return toolCalls, nil
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
				ToolCallID: tool.ToolCallId,
			}
			ylog.Debug("======== add toolMessage", "tm", fmt.Sprintf("%+v", tm))
			messages = append(messages, tm)
		}
	}

	// 3. user instruction
	messages = append(messages, openai.ChatCompletionMessage{Role: "user", Content: userInstruction})

	return messages
}
