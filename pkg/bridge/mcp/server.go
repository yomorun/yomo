package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
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
func Start(config *Config, aiConfig *pkgai.Config, zipperAddr string, log *slog.Logger) error {
	// ai provider
	provider, err := provider.GetProvider(aiConfig.Server.Provider)
	if err != nil {
		return err
	}
	// logger
	logger = log.With("service", "mcp-bridge")
	// ai service
	opts := &pkgai.ServiceOptions{
		Logger: logger,
		// SourceBuilder:  func(_ string) yomo.Source { return source },
		// ReducerBuilder: func(_ string) yomo.StreamFunction { return reducer },
	}
	zipperAddr = pkgai.ParseZipperAddr(zipperAddr)
	sourceBuilder := func(credential string) yomo.Source {
		source := yomo.NewSource("mcp-source", zipperAddr, yomo.WithCredential(credential))
		return source
	}
	reducerBuilder := func(credential string) yomo.StreamFunction {
		reducer := yomo.NewStreamFunction("mcp-reducer", zipperAddr, yomo.WithSfnCredential(credential))
		return reducer
	}
	opts.SourceBuilder = sourceBuilder
	opts.ReducerBuilder = reducerBuilder
	aiService = pkgai.NewService(provider, opts)
	// http server
	addr := config.Server.Addr
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/sse", mcpServerHandler)
	mux.HandleFunc("/message", mcpServerHandler)
	httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	// mcp server
	mcpServer, err = NewMCPServer(logger)
	if err != nil {
		logger.Error("[mcp] failed to create server", "error", err)
		return err
	}
	logger.Info("[mcp] mcp bridge server is up and running", "endpoint", fmt.Sprintf("http://%s/sse", addr))
	defer httpServer.Close()

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

func mcpServerHandler(w http.ResponseWriter, r *http.Request) {
	if mcpServer == nil {
		// mpc server is disabled
		w.WriteHeader(http.StatusNotFound)
		return
	}
	mcpServer.ServeHTTP(w, r)
}

// AddMCPTool add mcp tool
func AddMCPTool(connID uint64, functionDefinition *openai.FunctionDefinition) error {
	if mcpServer == nil {
		// mpc server is disabled
		return nil
	}
	// add tool
	tool := mcp.NewToolWithRawSchema(
		functionDefinition.Name,
		functionDefinition.Description,
		json.RawMessage(`{}`),
	)
	// add input schema
	if functionDefinition.Parameters != nil {
		inputSchema, err := json.Marshal(functionDefinition.Parameters)
		if err != nil {
			return err
		}
		tool.RawInputSchema = json.RawMessage(inputSchema)
	}
	// Add tool handler
	mcpServer.AddTool(tool, mcpToolHandler)
	tools.Store(connID, functionDefinition)
	logger.Info("[mcp] add tool", "input_schema", string(tool.RawInputSchema), "conn_id", connID)

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
func mcpToolHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	callResult, err := caller.Call(ctx, transID, reqID, fnCalls)
	if err != nil {
		logger.Error("[mcp] tool call error", "error", err, "name", name, "arguments", args)
		return nil, err
	}
	result := callResult[0].Content
	logger.Info("[mcp] tool call result", "name", name, "arguments", args, "result", string(result))

	return mcp.NewToolResultText(result), nil
}
