package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/ylog"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel"
)

var (
	ErrMCPServerNotFound    = errors.New("mcp server not found")
	ErrUnknownMCPServerType = errors.New("unknown mcp server type")
	ErrCallerNotFound       = errors.New("caller not found")
	ErrTracerNotFound       = errors.New("tracer not found")
)

// MCPServer represents a MCP server
type MCPServer struct {
	underlying            *mcp.Server
	SSEHandler            http.Handler
	StreamableHTTPHandler http.Handler
	logger                *slog.Logger
}

// NewMCPServer create a new mcp server
func NewMCPServer(logger *slog.Logger) (*MCPServer, error) {
	// logger
	if logger == nil {
		logger = ylog.Default()
	}
	// create mcp server
	opts := &mcp.ServerOptions{Logger: logger}
	underlyingMCPServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-server",
			Version: "2025-03-26",
		},
		opts,
	)
	underlyingMCPServer.AddReceivingMiddleware(loggingMiddleware(logger))
	// sse handler
	sseHandler := mcp.NewSSEHandler(
		func(request *http.Request) *mcp.Server {
			return underlyingMCPServer
		},
		&mcp.SSEOptions{},
	)
	// streamable http handler
	streamableHTTPHandler := mcp.NewStreamableHTTPHandler(
		func(request *http.Request) *mcp.Server {
			return underlyingMCPServer
		},
		&mcp.StreamableHTTPOptions{
			Logger: logger,
		},
	)
	// mcp server
	mcpServer := &MCPServer{
		underlying:            underlyingMCPServer,
		SSEHandler:            authHandler(sseHandler),
		StreamableHTTPHandler: authHandler(streamableHTTPHandler),
		logger:                logger,
	}

	logger.Info("[mcp] create mcp server",
		"sse_endpoint", "/sse",
		"streamable_http_endpoint", "/mcp",
	)

	// add prompt
	mcpServer.AddPrompt(&mcp.Prompt{Name: "yomo"}, promptHandler)

	return mcpServer, nil
}

// AddTool adds a tool to the mcp server
func (s *MCPServer) AddTool(tool *mcp.Tool, handler mcp.ToolHandler) {
	s.underlying.AddTool(tool, handler)
}

// DeleteTools deletes tools by name
func (s *MCPServer) DeleteTools(names ...string) {
	s.underlying.RemoveTools(names...)
}

// AddPrompt adds a prompt to the mcp server
func (s *MCPServer) AddPrompt(prompt *mcp.Prompt, handler mcp.PromptHandler) {
	s.underlying.AddPrompt(prompt, handler)
}

// promptHandler returns a prompt handler
func promptHandler(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	// get caller
	caller := pkgai.FromCallerContext(ctx)
	if caller == nil {
		logger.Error("[mcp] prompt handler load failed", "error", ErrCallerNotFound.Error())
		return nil, ErrCallerNotFound
	}
	// get prompt
	systemPrompt, op := caller.GetSystemPrompt()
	logger.Info("[mcp] add prompt", "name", request.Params.Name, "prompt", systemPrompt, "operation", op)

	return &mcp.GetPromptResult{
		Description: "yomo system prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role: "assistant",
				Content: &mcp.TextContent{
					Text: systemPrompt,
				},
			},
		},
	}, nil
}

// authHandler mcp auth handler
func authHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// trace
		ctx = pkgai.WithTracerContext(ctx, otel.Tracer("yomo-mcp-bridge"))
		// context with caller
		caller, err := aiService.LoadOrCreateCaller(r)
		if err != nil {
			logger.Error("[mcp] failed to load or create caller", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("failed to load caller"))
			return
		}
		// TEST: set system prompt for testing
		// caller.SetSystemPrompt("You are a helpful assistant.", pkgai.SystemPromptOpPrefix)
		// caller
		ctx = pkgai.WithCallerContext(ctx, caller)
		r = r.WithContext(ctx)
		// handle request
		handler.ServeHTTP(w, r)
	})
}

// toolHandler mcp tool handler
func toolHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// get tracer
	tracer := pkgai.FromTracerContext(ctx)
	if tracer == nil {
		logger.Error("[mcp] tool handler load failed", "error", ErrTracerNotFound.Error())
		return nil, ErrTracerNotFound
	}

	// get caller
	caller := pkgai.FromCallerContext(ctx)
	if caller == nil {
		logger.Error("[mcp] tool handler load failed", "error", ErrCallerNotFound.Error())
		return nil, ErrCallerNotFound
	}
	// run sfn and get result
	transID := id.New(32)
	reqID := id.New(16)
	toolCallID := id.New(8)
	name := request.Params.Name
	arguments, err := json.Marshal(request.Params.Arguments)
	if err != nil {
		return nil, err
	}
	args := string(arguments)
	logger.Info("[mcp] tool is calling...", "name", name, "arguments", args)
	fnCalls := []openai.ToolCall{
		{
			ID:   toolCallID,
			Type: "function",
			Function: openai.FunctionCall{
				Name:      name,
				Arguments: string(arguments),
			},
		},
	}
	agentContext := request.Params.Meta.GetMeta()

	callResult, err := caller.Call(ctx, transID, reqID, agentContext, fnCalls, tracer)
	if err != nil {
		logger.Error("[mcp] tool call error", "error", err, "name", name, "arguments", args)
		return nil, err
	}
	result := callResult[0].Content
	logger.Info("[mcp] tool call result", "name", name, "arguments", args, "result", string(result))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(result),
			},
		},
	}, nil
}

func loggingMiddleware(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			args := []any{
				"session_id", req.GetSession().ID(),
				"has_params", req.GetParams() != nil,
			}
			// call tool request logging
			callToolReq, isCallToolReq := req.(*mcp.CallToolRequest)
			if isCallToolReq {
				args = append(
					args,
					"name", callToolReq.Params.Name,
					"arguments", string(callToolReq.Params.Arguments),
				)
			}
			logger.With(args...).Debug(fmt.Sprintf("[mcp] rpc:%s started", method))
			start := time.Now()
			result, err := next(ctx, method, req)
			duration := time.Since(start)
			// call tool result logging
			callToolResult, isCallToolResult := result.(*mcp.CallToolResult)
			if isCallToolResult {
				if callToolResult != nil {
					content, _ := json.Marshal(callToolResult.Content)
					args = append(
						args,
						"content", string(content),
						"structured_content", callToolResult.StructuredContent,
					)
				}
			}
			if err != nil {
				logger.With(args...).Error(
					fmt.Sprintf("[mcp] rpc:%s failed", method),
					"duration_ms", duration.Milliseconds(),
					"err", err,
				)
			} else {
				logger.With(args...).Debug(
					fmt.Sprintf("[mcp] rpc:%s completed", method),
					"duration_ms", duration.Milliseconds(),
					"has_result", result != nil,
				)
			}
			return result, err
		}
	}
}
