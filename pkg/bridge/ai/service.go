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
	credential string
	zipperAddr string
	md         metadata.M
	source     yomo.Source
	reducer    yomo.StreamFunction
	cache      map[string]*CacheItem
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
		credential:  credential,
		zipperAddr:  zipperAddr,
		cache:       make(map[string]*CacheItem),
		LLMProvider: aiProvider,
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
		ylog.Debug("<< ai-reducer", "tag", ai.ReducerTag, "data", string(buf))
		invoke, err := ai.ParseFunctionCallContext(ctx)
		if err != nil {
			ylog.Error("parse function calling invoke", "err", err.Error())
			return
		}
		reqID := invoke.ReqID
		v, ok := s.cache[reqID]
		if !ok {
			ylog.Error("req_id not found", "req_id", reqID)
			return
		}
		defer v.wg.Done()

		v.mu.Lock()
		defer v.mu.Unlock()

		fmt.Fprintf(v.ResponseWriter, "event: result\n")
		fmt.Fprintf(v.ResponseWriter, "data: %s\n\n", invoke.JSONString())
		// fmt.Fprintf(v.ResponseWriter, "event: retrieval_result\n")
		// fmt.Fprintf(v.ResponseWriter, "data: %s\n\n", invoke.RetrievalResult)

		// // one json per line, like groq.com did
		// fmt.Fprintf(v.ResponseWriter, invoke.JSONString()+"\n")
		// fmt.Fprintf(v.ResponseWriter, "{\"retrievalData\": \"%s\"}\n", invoke.RetrievalResult)

		// flush the response
		flusher, ok := v.ResponseWriter.(http.Flusher)
		if ok {
			flusher.Flush()
		}
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
func (s *Service) GetChatCompletions(userInstruction string) (*ai.InvokeResponse, error) {
	// messages
	baseSystemMessage := `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

	// we do not support multi-turn invoke for Google Gemini
	if s.LLMProvider.Name() == "gemini" {
		return s.LLMProvider.GetChatCompletions(userInstruction, baseSystemMessage, nil, s.md)
	} else {
		return s.execute(userInstruction)
	}
}

func (s *Service) execute(userInstruction string) (*ai.InvokeResponse, error) {
	// messages
	baseSystemMessage := `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

	res, err := s.LLMProvider.GetChatCompletions(userInstruction, baseSystemMessage, nil, s.md)
	if err != nil {
		return nil, err
	}

	if res.FinishReason == "tools_call" {
		res1, err := s.LLMProvider.GetChatCompletions(userInstruction, baseSystemMessage, res.ToolCalls[0], s.md)
		if err != nil {
			return nil, err
		}
		return res1, nil
	}

	return res, nil
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
