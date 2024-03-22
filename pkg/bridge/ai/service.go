package ai

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
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
	// TODO: this cache can be removed as the BasicAPIServer only contains 1 service instance.
	services *expirable.LRU[string, *Service]
)

// CacheItem cache the http.ResponseWriter, which is used for writing response from reducer.
// TODO: http.ResponseWriter is from the SimpleRestfulServer interface, should be decoupled
// from here.
type CacheItem struct {
	ResponseWriter http.ResponseWriter
	wg             *sync.WaitGroup
	mu             sync.Mutex
}

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
	cache        map[string]*CacheItem
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
		credential:   credential,
		zipperAddr:   zipperAddr,
		cache:        make(map[string]*CacheItem),
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
	clear(s.cache)
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

	functions := make(map[uint32]*ai.FunctionDefinition)
	for tag, tc := range tcs {
		functions[tag] = tc.Function
	}

	return &ai.OverviewResponse{Functions: functions}, nil
}

// GetChatCompletions returns the llm api response
func (s *Service) GetChatCompletions(userInstruction string, baseSystemMessage string, reqID string, includeCallStack bool) (*ai.InvokeResponse, error) {
	chainMessage := ai.ChainMessage{}
	// we do not support multi-turn invoke for Google Gemini
	if s.LLMProvider.Name() == "gemini" {
		return s.LLMProvider.GetChatCompletions(userInstruction, baseSystemMessage, chainMessage, s.md, true)
	}

	res, err := s.LLMProvider.GetChatCompletions(userInstruction, baseSystemMessage, chainMessage, s.md, true)
	if err != nil {
		return nil, err
	}

	// if no tool_calls fired, just return the llm text result
	if res.FinishReason != "tool_calls" {
		return res, nil
	}

	// run llm function calls
	llmCalls, err := s.runFunctionCalls(res.ToolCalls, reqID)
	if err != nil {
		return nil, err
	}

	ylog.Debug(">>>> start 2nd call with", "calls", fmt.Sprintf("%+v", llmCalls), "preceeding_assistant_message", fmt.Sprintf("%+v", res.AssistantMessage))
	chainMessage.PreceedingAssistantMessage = res.AssistantMessage
	chainMessage.ToolMessages = llmCalls
	// do not attach toolMessage to prompt in 2nd call
	res2, err := s.LLMProvider.GetChatCompletions(userInstruction, baseSystemMessage, chainMessage, s.md, false)
	// INFO: call stack infomation
	if includeCallStack {
		res2.ToolCalls = res.ToolCalls
		res2.ToolMessages = llmCalls
	}
	ylog.Debug("<<<< complete 2nd call", "res2", fmt.Sprintf("%+v", res2))

	return res2, err
}

// run llm-sfn function calls
func (s *Service) runFunctionCalls(fns map[uint32][]*ai.ToolCall, reqID string) ([]ai.ToolMessage, error) {
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
			asyncCall.wg.Add(1)
		}
	}

	// wait for reducer to finish, the aggregation results
	asyncCall.wg.Wait()

	arr := make([]ai.ToolMessage, 0)

	for _, call := range asyncCall.val {
		ylog.Debug("---invoke done", "id", call.ToolCallId, "content", call.Content)
		call.Role = "tool"
		arr = append(arr, call)
	}

	return arr, nil
}

// fireLlmSfn fires the llm-sfn function call by s.source.Write()
func (s *Service) fireLlmSfn(tag uint32, fn *ai.ToolCall, reqID string) error {
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
	services = expirable.NewLRU[string, *Service](ServiceCacheSize, onEvicted, ServiceCacheTTL)
}

type sfnAsyncCall struct {
	wg  *sync.WaitGroup
	mu  sync.Mutex
	val map[string]ai.ToolMessage
}
