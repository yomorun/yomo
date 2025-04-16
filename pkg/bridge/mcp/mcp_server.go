package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai"
)

var (
	ErrMCPServerNotFound    = errors.New("mcp server not found")
	ErrUnknownMCPServerType = errors.New("unknown mcp server type")
	ErrCallerNotFound       = errors.New("caller not found")
)

// MCPServer represents a MCP server
type MCPServer struct {
	underlying *server.MCPServer
	SSEServer  *server.SSEServer
	basePath   string
	logger     *slog.Logger
}

// NewMCPServer create a new mcp server
func NewMCPServer(logger *slog.Logger) (*MCPServer, error) {
	// logger
	if logger == nil {
		logger = ylog.Default()
	}
	// create mcp server
	underlyingMCPServer := server.NewMCPServer(
		"mcp-server",
		"2024-11-05",
		server.WithLogging(),
		server.WithHooks(hooks(logger)),
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
	// sse options
	sseOpts := []server.SSEOption{
		server.WithHTTPServer(httpServer),
		server.WithSSEContextFunc(authContextFunc()),
	}
	// sse server
	sseServer := server.NewSSEServer(underlyingMCPServer, sseOpts...)

	mcpServer := &MCPServer{
		underlying: underlyingMCPServer,
		SSEServer:  sseServer,
		logger:     logger,
	}

	logger.Info("[mcp] server is created",
		"sse_endpoint", sseServer.CompleteSseEndpoint(),
		"message_endpoint", sseServer.CompleteMessageEndpoint(),
	)

	return mcpServer, nil
}

// BasePath returns the base path of the mcp server
func (s *MCPServer) BasePath() string {
	return s.basePath
}

func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Info(fmt.Sprintf("[mcp] url:%s", r.URL.String()), "method", r.Method)
	s.SSEServer.ServeHTTP(w, r)
}

// AddTool adds a tool to the mcp server
func (s *MCPServer) AddTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	s.underlying.AddTool(tool, handler)
}

// DeleteTools deletes tools by name
func (s *MCPServer) DeleteTools(names ...string) {
	s.underlying.DeleteTools(names...)
}

// AddPrompt adds a prompt to the mcp server
func (s *MCPServer) AddPrompt(prompt mcp.Prompt, handler server.PromptHandlerFunc) {
	s.underlying.AddPrompt(prompt, handler)
}

func authContextFunc() server.SSEContextFunc {
	return func(ctx context.Context, r *http.Request) context.Context {
		// context with caller
		caller, err := aiService.LoadOrCreateCaller(r)
		if err != nil {
			logger.Error("[mcp] failed to load or create caller", "error", err)
			return ctx
		}
		// caller
		ctx = ai.WithCallerContext(ctx, caller)
		logger.Debug("[mcp] sse context with caller", "path", r.URL.Path)
		return ctx
	}
}

func hooks(logger *slog.Logger) *server.Hooks {
	hooks := &server.Hooks{}

	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		logger.Debug("[mcp] hook.beforeAny", "method", method, "id", id, "message", message)
	})
	hooks.AddOnSuccess(func(ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
		logger.Info(fmt.Sprintf("[mcp] rpc:%s", method), "id", id, "message", message, "result", result)
	})
	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		logger.Error("[mcp] rpc call error", "method", method, "id", id, "message", message, "error", err)
	})
	// initialize
	hooks.AddBeforeInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest) {
		logger.Debug("[mcp] hook.beforeInitialize", "id", id, "message", message)
	})
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		logger.Debug("[mcp] hook.afterInitialize", "id", id, "message", message, "result", result)
	})
	// ping
	hooks.AddBeforePing(func(ctx context.Context, id any, message *mcp.PingRequest) {
		logger.Debug("[mcp] hook.beforePing", "id", id, "message", message)
	})
	hooks.AddAfterPing(func(ctx context.Context, id any, message *mcp.PingRequest, result *mcp.EmptyResult) {
		logger.Debug("[mcp] hook.afterPing", "id", id, "message", message, "result", result)
	})
	// list resources
	hooks.AddBeforeListResources(func(ctx context.Context, id any, message *mcp.ListResourcesRequest) {
		logger.Debug("[mcp] hook.beforeListResources", "id", id, "message", message)
	})
	hooks.AddAfterListResources(func(ctx context.Context, id any, message *mcp.ListResourcesRequest, result *mcp.ListResourcesResult) {
		logger.Debug("[mcp] hook.afterListResources", "id", id, "message", message, "result", result)
	})
	// list resource templates
	hooks.AddBeforeListResourceTemplates(func(ctx context.Context, id any, message *mcp.ListResourceTemplatesRequest) {
		logger.Debug("[mcp] hook.beforeListResourceTemplates", "id", id, "message", message)
	})
	hooks.AddAfterListResourceTemplates(func(ctx context.Context, id any, message *mcp.ListResourceTemplatesRequest, result *mcp.ListResourceTemplatesResult) {
		logger.Debug("[mcp] hook.afterListResourceTemplates", "id", id, "message", message, "result", result)
	})
	// read resource
	hooks.AddBeforeReadResource(func(ctx context.Context, id any, message *mcp.ReadResourceRequest) {
		logger.Debug("[mcp] hook.beforeReadResource", "id", id, "message", message)
	})
	hooks.AddAfterReadResource(func(ctx context.Context, id any, message *mcp.ReadResourceRequest, result *mcp.ReadResourceResult) {
		logger.Debug("[mcp] hook.afterReadResource", "id", id, "message", message, "result", result)
	})
	// list prompts
	hooks.AddBeforeListPrompts(func(ctx context.Context, id any, message *mcp.ListPromptsRequest) {
		logger.Debug("[mcp] hook.beforeListPrompts", "id", id, "message", message)
	})
	hooks.AddAfterListPrompts(func(ctx context.Context, id any, message *mcp.ListPromptsRequest, result *mcp.ListPromptsResult) {
		logger.Debug("[mcp] hook.afterListPrompts", "id", id, "message", message, "result", result)
	})
	// get prompt
	hooks.AddBeforeGetPrompt(func(ctx context.Context, id any, message *mcp.GetPromptRequest) {
		logger.Debug("[mcp] hook.beforeGetPrompt", "id", id, "message", message)
	})
	hooks.AddAfterGetPrompt(func(ctx context.Context, id any, message *mcp.GetPromptRequest, result *mcp.GetPromptResult) {
		logger.Debug("[mcp] hook.afterGetPrompt", "id", id, "message", message, "result", result)
	})
	// list tools
	hooks.AddBeforeListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest) {
		logger.Debug("[mcp] hook.beforeListTools", "id", id, "message", message)
	})
	hooks.AddAfterListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		logger.Debug("[mcp] hook.afterListTools", "id", id, "message", message, "result", result)
	})
	// call tool
	hooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {
		logger.Debug("[mcp] hook.beforeCallTool", "id", id, "message", message)
	})
	hooks.AddAfterCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest, result *mcp.CallToolResult) {
		logger.Debug("[mcp] hook.afterCallTool", "id", id, "message", message, "result", result)
	})

	return hooks
}
