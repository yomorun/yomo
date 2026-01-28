package ai

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/caller"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Service is the  service layer for llm bridge server.
// service is responsible for handling the logic from handler layer.
type Service struct {
	provider     provider.LLMProvider
	createCaller caller.CreateCallerFunc
	callers      *expirable.LRU[string, *caller.Caller]
	option       *ServiceOptions
	logger       *slog.Logger
}

// ServiceOptions is the option for creating service
type ServiceOptions struct {
	// Logger is the logger for the service
	Logger *slog.Logger
	// CredentialFunc is the function for getting the credential from the request
	CredentialFunc func(r *http.Request) (string, error)
	// CallerCacheSize is the size of the caller's cache
	CallerCacheSize int
	// CallerCacheTTL is the time to live of the callers cache
	CallerCacheTTL time.Duration
	// CallerCallTimeout is the timeout for awaiting the function response.
	CallerCallTimeout time.Duration
	// SourceBuilder should builds an unconnected source.
	SourceBuilder func(credential string) yomo.Source
	// ReducerBuilder should builds an unconnected reducer.
	ReducerBuilder func(credential string) yomo.StreamFunction
	// MetadataExchanger exchanges metadata from the credential.
	MetadataExchanger func(credential string) (metadata.M, error)
}

// NewService creates a new service for handling the logic from handler layer.
func NewService(provider provider.LLMProvider, opt *ServiceOptions) *Service {
	return NewServiceWithCallerFunc(provider, caller.NewCaller, opt)
}

func initOption(opt *ServiceOptions) *ServiceOptions {
	if opt == nil {
		opt = &ServiceOptions{}
	}
	if opt.Logger == nil {
		opt.Logger = ylog.Default()
	}
	if opt.CredentialFunc == nil {
		opt.CredentialFunc = func(_ *http.Request) (string, error) { return "token", nil }
	}
	if opt.CallerCacheSize == 0 {
		opt.CallerCacheSize = 1
	}
	if opt.CallerCallTimeout == 0 {
		opt.CallerCallTimeout = 60 * time.Second
	}
	if opt.MetadataExchanger == nil {
		opt.MetadataExchanger = func(credential string) (metadata.M, error) {
			return metadata.New(), nil
		}
	}

	return opt
}

func NewServiceWithCallerFunc(provider provider.LLMProvider, createCaller caller.CreateCallerFunc, opt *ServiceOptions) *Service {
	onEvict := func(_ string, caller *caller.Caller) {
		caller.Close()
	}

	opt = initOption(opt)

	service := &Service{
		provider:     provider,
		createCaller: createCaller,
		callers:      expirable.NewLRU(opt.CallerCacheSize, onEvict, opt.CallerCacheTTL),
		option:       opt,
		logger:       opt.Logger,
	}

	return service
}

// LoadOrCreateCaller loads or creates the caller according to the http request.
func (srv *Service) LoadOrCreateCaller(r *http.Request) (*caller.Caller, error) {
	credential, err := srv.option.CredentialFunc(r)
	if err != nil {
		return nil, err
	}
	return srv.loadOrCreateCaller(credential)
}

// GetInvoke returns the invoke response
func (srv *Service) GetInvoke(ctx context.Context, userInstruction, transID string, caller *caller.Caller, includeCallStack bool, agentContext []byte, w EventResponseWriter, tracer trace.Tracer) error {
	if tracer == nil {
		tracer = new(noop.Tracer)
	}

	// 1. tag this call is a invoke call
	md := caller.Metadata().Clone()
	if md == nil {
		md = metadata.New()
	}
	md.Set(invokeMetadataKey, "1")
	md.Set(invokeIncludeCallStackMetadataKey, fmt.Sprintf("%v", includeCallStack))

	// 2. create the chat completion request
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: userInstruction},
		},
		Stream: false,
	}

	// 3. find all hosting tool sfn
	tools, err := ai.ListToolCalls(md)
	if err != nil {
		return err
	}

	// 2. add those tools to request
	req, hasReqTools := srv.addToolsToRequest(req, tools)

	// 3. operate system prompt to request
	prompt, op := caller.GetSystemPrompt()
	req = srv.OpSystemPrompt(req, prompt, op)

	// 4. loop if multi-turn function calling until call stop
	w.RecordIsStream(req.Stream)
	if err := multiTurnFunctionCalling(ctx, req, transID, hasReqTools, w, srv.provider, caller, tracer, md, agentContext); err != nil {
		srv.logger.Error("chatCompletionFailed", "transID", transID, "err", err)
		return err
	}
	return nil
}

// GetChatCompletions accepts openai.ChatCompletionRequest and responds to http.ResponseWriter.
func (srv *Service) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, transID string, agentContext []byte, caller *caller.Caller, w EventResponseWriter, tracer trace.Tracer) error {
	if tracer == nil {
		tracer = new(noop.Tracer)
	}
	md := caller.Metadata().Clone()
	if md == nil {
		md = metadata.New()
	}

	// 1. find all hosting tool sfn
	tools, err := ai.ListToolCalls(md)
	if err != nil {
		return err
	}

	// 2. add those tools to request
	req, hasReqTools := srv.addToolsToRequest(req, tools)

	// 3. operate system prompt to request
	prompt, op := caller.GetSystemPrompt()
	req = srv.OpSystemPrompt(req, prompt, op)

	// 4. loop if multi-turn function calling until call stop
	w.RecordIsStream(req.Stream)
	if err := multiTurnFunctionCalling(ctx, req, transID, hasReqTools, w, srv.provider, caller, tracer, md, agentContext); err != nil {
		srv.logger.Error("chatCompletionFailed", "transID", transID, "err", err)
		return err
	}
	return nil
}

func (srv *Service) loadOrCreateCaller(credential string) (*caller.Caller, error) {
	caller, ok := srv.callers.Get(credential)
	if ok {
		return caller, nil
	}
	md, err := srv.option.MetadataExchanger(credential)
	if err != nil {
		return nil, err
	}
	caller, err = srv.createCaller(
		srv.option.SourceBuilder(credential),
		srv.option.ReducerBuilder(credential),
		md,
		srv.option.CallerCallTimeout,
	)
	if err != nil {
		return nil, err
	}

	srv.callers.Add(credential, caller)

	return caller, nil
}

func (srv *Service) addToolsToRequest(req openai.ChatCompletionRequest, tools []openai.Tool) (openai.ChatCompletionRequest, bool) {
	hasReqTools := len(req.Tools) > 0
	if !hasReqTools {
		if len(tools) > 0 {
			req.Tools = tools
			srv.logger.Debug("#1 first call", "request", fmt.Sprintf("%+v", req))
		}
	}
	return req, hasReqTools
}

func (srv *Service) OpSystemPrompt(req openai.ChatCompletionRequest, sysPrompt string, op caller.SystemPromptOp) openai.ChatCompletionRequest {
	if op == caller.SystemPromptOpDisabled {
		return req
	}
	if op == caller.SystemPromptOpOverwrite && sysPrompt == "" {
		return req
	}
	var (
		systemCount = 0
		messages    = []openai.ChatCompletionMessage{}
	)
	for _, msg := range req.Messages {
		if msg.Role != "system" {
			messages = append(messages, msg)
			continue
		}
		if systemCount == 0 {
			content := ""
			switch op {
			case caller.SystemPromptOpPrefix:
				content = sysPrompt + "\n" + msg.Content
			case caller.SystemPromptOpOverwrite:
				content = sysPrompt
			case caller.SystemPromptOpClientPreferred:
				content = msg.Content
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: content,
			})
		}
		systemCount++
	}

	if systemCount == 0 && sysPrompt != "" {
		if op == caller.SystemPromptOpClientPreferred || op == caller.SystemPromptOpOverwrite || op == caller.SystemPromptOpPrefix {
			message := openai.ChatCompletionMessage{
				Role:    "system",
				Content: sysPrompt,
			}
			messages = append([]openai.ChatCompletionMessage{message}, req.Messages...)
		}
	}
	req.Messages = messages

	srv.logger.Debug(" #1 first call after operating", "request", fmt.Sprintf("%+v", req))

	return req
}

type callerContextKey struct{}

// WithCallerContext adds the caller to the request context
func WithCallerContext(ctx context.Context, caller *caller.Caller) context.Context {
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// FromCallerContext returns the caller from the request context
func FromCallerContext(ctx context.Context) *caller.Caller {
	caller, ok := ctx.Value(callerContextKey{}).(*caller.Caller)
	if !ok {
		return nil
	}
	return caller
}

type transIDContextKey struct{}

// WithTransIDContext adds the transID to the request context
func WithTransIDContext(ctx context.Context, transID string) context.Context {
	return context.WithValue(ctx, transIDContextKey{}, transID)
}

// FromTransIDContext returns the transID from the request context
func FromTransIDContext(ctx context.Context) string {
	val, ok := ctx.Value(transIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return val
}

type tracerContextKey struct{}

// WithTracerContext adds the tracer to the request context
func WithTracerContext(ctx context.Context, tracer trace.Tracer) context.Context {
	return context.WithValue(ctx, tracerContextKey{}, tracer)
}

// FromTracerContext returns the tracer from the request context
func FromTracerContext(ctx context.Context) trace.Tracer {
	val, ok := ctx.Value(tracerContextKey{}).(trace.Tracer)
	if !ok {
		return new(noop.Tracer)
	}
	return val
}
