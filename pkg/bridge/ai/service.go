package ai

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

var services sync.Map

type CacheItem struct {
	ResponseWriter http.ResponseWriter
	wg             *sync.WaitGroup
	mu             sync.Mutex
}

type Service struct {
	credential string
	zipperAddr string
	source     yomo.Source
	reducer    yomo.StreamFunction
	reducerTag uint32
	cache      map[string]*CacheItem
	AIProvider
}

func NewService(credential string, zipperAddr string, reducerTag uint32, aiProvider AIProvider) (*Service, error) {
	val, ok := services.Load(credential)
	if ok {
		return val.(*Service), nil
	}
	s, err := newService(credential, zipperAddr, reducerTag, aiProvider)
	if err != nil {
		ylog.Error("create AI service failed", "err", err)
		return nil, err
	}
	services.Store(credential, s)
	return s, nil
}

func newService(credential string, zipperAddr string, reducerTag uint32, aiProvider AIProvider) (*Service, error) {
	s := &Service{
		credential: credential,
		zipperAddr: zipperAddr,
		reducerTag: reducerTag,
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
	s.source.Close()
	s.reducer.Close()
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
	sfn.SetObserveDataTags(s.reducerTag)
	sfn.SetHandler(func(ctx serverless.Context) {
		buf := ctx.Data()
		ylog.Debug("<< ai-reducer", "tag", s.reducerTag, "data", string(buf))
		call, err := ai.NewFunctionCallingInvoke(ctx)
		if err != nil {
			ylog.Error("parse function calling invoke", "err", err.Error())
			return
		}
		reqID := call.ReqID
		v, ok := s.cache[reqID]
		if !ok {
			ylog.Error("req_id not found", "req_id", reqID)
			return
		}
		defer v.wg.Done()

		v.mu.Lock()
		defer v.mu.Unlock()

		fmt.Fprintf(v.ResponseWriter, "data: %s\n\n", call.Arguments)
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
