package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/id"
)

var (
	mcpServer  *MCPServer
	tools      sync.Map
	httpServer *http.Server
	aiService  *pkgai.Service
	logger     *slog.Logger
)

// Start starts the http server
func Start(config *Config, aiConfig *pkgai.Config, source yomo.Source, reducer yomo.StreamFunction, log *slog.Logger) error {
	// ai provider
	provider, err := provider.GetProvider(aiConfig.Server.Provider)
	if err != nil {
		return err
	}
	// logger
	logger = log.With("service", "mcp-bridge")
	// ai service
	opts := &pkgai.ServiceOptions{
		Logger:         logger,
		SourceBuilder:  func(_ string) yomo.Source { return source },
		ReducerBuilder: func(_ string) yomo.StreamFunction { return reducer },
	}
	// zipperAddr = pkgai.ParseZipperAddr(zipperAddr)
	// sourceBuilder := func(credential string) yomo.Source {
	// 	source := yomo.NewSource("mcp-source", zipperAddr, yomo.WithCredential(credential))
	// 	return source
	// }
	// reducerBuilder := func(credential string) yomo.StreamFunction {
	// 	reducer := yomo.NewStreamFunction("mcp-reducer", zipperAddr, yomo.WithSfnCredential(credential))
	// 	return reducer
	// }
	// opts.SourceBuilder = sourceBuilder
	// opts.ReducerBuilder = reducerBuilder
	aiService = pkgai.NewService(provider, opts)

	// mcp server
	mcpServer, err = NewMCPServer(logger)
	if err != nil {
		logger.Error("[mcp] failed to create server", "error", err)
		return err
	}
	// http server
	mux := http.NewServeMux()
	addr := config.Server.Addr
	// handlers
	mux.HandleFunc("/", index)
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/sse", mcpServer.SSEServer.ServeHTTP)
	httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	logger.Info("[mcp] server is up and running", "endpoint", fmt.Sprintf("http://%s", addr))

	return httpServer.ListenAndServe()
}

// Stop stops the http server
func Stop() error {
	return httpServer.Close()
}

func index(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("MCP Server is running"))
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// AddMCPTool add mcp tool
func AddMCPTool(connID uint64, functionDefinition *openai.FunctionDefinition) error {
	if mcpServer == nil {
		// mpc server is disabled
		return nil
	}
	var err error
	// add tool
	rawInputSchema := []byte(`{"type":"object"}`)
	tool := &mcp.Tool{
		Name:        functionDefinition.Name,
		Description: functionDefinition.Description,
		// InputSchema: &jsonschema.Schema{Type: "object"},
		InputSchema: json.RawMessage(rawInputSchema),
		// json.RawMessage(`{}`),
	}
	// add input schema
	if functionDefinition.Parameters != nil {
		rawInputSchema, err = json.Marshal(functionDefinition.Parameters)
		if err != nil {
			return err
		}
		// tool.RawInputSchema = json.RawMessage(inputSchema)
		tool.InputSchema = json.RawMessage(rawInputSchema)
	}
	// Add tool handler
	mcpServer.AddTool(tool, mcpToolHandler)
	tools.Store(connID, functionDefinition)
	logger.Info("[mcp] add tool", "input_schema", string(rawInputSchema), "conn_id", connID)

	return nil
}

// RemoveMCPTool remove mcp tool
func RemoveMCPTool(connID uint64) error {
	if mcpServer == nil {
		// mpc server is disabled
		return nil
	}
	tools.Delete(connID)
	tool, ok := tools.Load(connID)
	if !ok {
		// tool not found
		return nil
	}
	functionDefinition, ok := tool.(*openai.FunctionDefinition)
	if !ok {
		// tool not found
		return nil
	}
	mcpServer.DeleteTools(functionDefinition.Name)
	return nil
}

// mcpToolHandler mcp tool handler
// type ToolHandler func(context.Context, *CallToolRequest) (*CallToolResult, error)
func mcpToolHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	callResult, err := caller.Call(ctx, transID, reqID, fnCalls, tracer)
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
