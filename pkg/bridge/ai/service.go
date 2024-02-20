package ai

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

var (
	// ServiceCacheSize is the size of the service cache
	ServiceCacheSize = 1024
	// ServiceCacheTTL is the time to live of the service cache
	ServiceCacheTTL = time.Minute * 30
	services        *expirable.LRU[string, *Service]
)

type CacheItem struct {
	ResponseWriter http.ResponseWriter
	wg             *sync.WaitGroup
	mu             sync.Mutex
}

// Service is the AI service
type Service struct {
	credential string
	zipperAddr string
	source     yomo.Source
	reducer    yomo.StreamFunction
	cache      map[string]*CacheItem
	AIProvider
}

// NewService creates a new AI service, if the service is already created, it will return the existing one
func NewService(credential string, zipperAddr string, aiProvider AIProvider) (*Service, error) {
	s, ok := services.Get(credential)
	if ok {
		return s, nil
	}
	s, err := newService(credential, zipperAddr, aiProvider)
	if err != nil {
		ylog.Error("create AI service failed", "err", err)
		return nil, err
	}
	services.Add(credential, s)
	return s, nil
}

func newService(credential string, zipperAddr string, aiProvider AIProvider) (*Service, error) {
	s := &Service{
		credential: credential,
		zipperAddr: zipperAddr,
		cache:      make(map[string]*CacheItem),
		AIProvider: aiProvider,
	}
	// source
	source, err := s.createSource()
	if err != nil {
		ylog.Error("create AI source failed", "err", err)
		return nil, err
	}
	s.source = source
	// reducer
	reducer, err := s.createReducer()
	if err != nil {
		ylog.Error("create AI reducer failed", "err", err)
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
	// INFO: use clear on go1.21 or later
	// clear(s.cache)
	s.cache = map[string]*CacheItem{}
}

func (s *Service) createSource() (yomo.Source, error) {
	source := yomo.NewSource(
		"ai-source",
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
		invoke, err := ai.NewFunctionCallingInvoke(ctx)
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

		fmt.Fprintf(v.ResponseWriter, "data: %s\n\n", invoke.Arguments)
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

func (s *Service) GetOverview() (*ai.OverviewResponse, error) {
	return s.AIProvider.GetOverview()
}

func (s *Service) GetChatCompletions(prompt string) (*ai.ChatCompletionsResponse, error) {
	return s.AIProvider.GetChatCompletions(prompt)
}

func (s *Service) Write(tag uint32, data []byte) error {
	return s.source.Write(tag, data)
}

func init() {
	onEvicted := func(k string, v *Service) {
		v.Release()
	}
	services = expirable.NewLRU[string, *Service](ServiceCacheSize, onEvicted, ServiceCacheTTL)
}
