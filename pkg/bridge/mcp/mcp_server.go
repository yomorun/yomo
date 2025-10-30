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
	"github.com/yomorun/yomo/core/ylog"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
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
	underlying           *mcp.Server
	SSEServer            http.Handler
	StreamableHTTPServer *mcp.StreamableHTTPHandler
	basePath             string
	logger               *slog.Logger
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
	// sse server
	sseServer := mcp.NewSSEHandler(
		func(request *http.Request) *mcp.Server {
			return underlyingMCPServer
		},
		&mcp.SSEOptions{},
	)

	mcpServer := &MCPServer{
		underlying: underlyingMCPServer,
		SSEServer:  authHandler(sseServer),
		logger:     logger,
	}

	logger.Info("[mcp] create mcp server",
		"sse_endpoint", "/sse",
		"streamable_http_endpoint", "/mcp",
	)

	return mcpServer, nil
}

// BasePath returns the base path of the mcp server
func (s *MCPServer) BasePath() string {
	return s.basePath
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
		// caller
		ctx = pkgai.WithCallerContext(ctx, caller)
		r = r.WithContext(ctx)
		// handle request
		handler.ServeHTTP(w, r)
	})
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
