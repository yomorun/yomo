package mcp

import (
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
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/sse", mcpServer.SSEHandler.ServeHTTP)
	mux.HandleFunc("/mcp", mcpServer.StreamableHTTPHandler.ServeHTTP)
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

func indexHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("MCP Server is running"))
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

type MCPToolStore struct {
}

// AddMCPTool add mcp tool
func (s *MCPToolStore) AddMCPTool(connID uint64, functionDefinition *openai.FunctionDefinition) error {
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
		InputSchema: json.RawMessage(rawInputSchema),
	}
	// add input schema
	if functionDefinition.Parameters != nil {
		rawInputSchema, err = json.Marshal(functionDefinition.Parameters)
		if err != nil {
			return err
		}
		tool.InputSchema = json.RawMessage(rawInputSchema)
	}
	// Add tool handler
	mcpServer.AddTool(tool, toolHandler)
	tools.Store(connID, functionDefinition)
	logger.Info("[mcp] add tool", "name", tool.Name, "input_schema", string(rawInputSchema), "conn_id", connID)

	return nil
}

// RemoveMCPTool remove mcp tool
func (s *MCPToolStore) RemoveMCPTool(connID uint64) error {
	if mcpServer == nil {
		// mpc server is disabled
		return nil
	}
	tool, ok := tools.Load(connID)
	if !ok {
		// tool not found
		return nil
	}
	tools.Delete(connID)
	functionDefinition, ok := tool.(*openai.FunctionDefinition)
	if !ok {
		// tool not found
		return nil
	}
	mcpServer.DeleteTools(functionDefinition.Name)
	logger.Info("[mcp] remove tool", "name", functionDefinition.Name, "conn_id", connID)
	return nil
}
