package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	baseProvider "github.com/yomorun/yomo/pkg/bridge/ai"
)

var fns sync.Map

type connectedFn struct {
	connID uint64
	tag    uint32
	fd     *FunctionDeclaration
}

func init() {
	fns = sync.Map{}
}

// GeminiProvider is the provider for Gemini
type GeminiProvider struct {
	APIKey string
}

var _ = baseProvider.LLMProvider(&GeminiProvider{})

// Name returns the name of the provider
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// GetChatCompletions get chat completions for ai service
func (p *GeminiProvider) GetChatCompletions(userInstruction string) (*ai.InvokeResponse, error) {
	// check if there are any tool calls attached, if no, return error
	isEmpty := true
	fns.Range(func(_, _ interface{}) bool {
		isEmpty = false
		return false
	})

	if isEmpty {
		ylog.Error("-----tools is empty")
		return &ai.InvokeResponse{Content: "no toolCalls"}, ai.ErrNoFunctionCall
	}

	// prepare request body
	body := p.prepareRequestBody(userInstruction)

	// request API
	jsonBody, err := json.Marshal(body)
	if err != nil {
		ylog.Error(err.Error())
		return nil, err
	}

	ylog.Debug("gemini api request", "body", string(jsonBody))

	req, err := http.NewRequest("POST", p.getApiUrl(), bytes.NewBuffer(jsonBody))
	if err != nil {
		ylog.Error(err.Error())
		fmt.Println("Error creating new request:", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ylog.Error(err.Error())
		fmt.Println("Error making request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		ylog.Error(err.Error())
		fmt.Println("Error reading response body:", err)
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini provider api response status code is %d", resp.StatusCode)
	}

	ylog.Debug("gemini api response", "body", string(respBody))

	// parse response
	response, err := parseAPIResponseBody(respBody)
	if err != nil {
		ylog.Error(err.Error())
		return nil, err
	}

	// get all candidates as []*ai.ToolCall
	calls := parseToolCallFromResponse(response)

	ylog.Debug("gemini api response", "calls", len(calls))

	result := &ai.InvokeResponse{}
	if len(calls) == 0 {
		return result, ai.ErrNoFunctionCall
	}

	result.ToolCalls = make(map[uint32][]*ai.ToolCall)
	for _, call := range calls {
		ylog.Debug("++call", "call", call.Function.Name, "call", call.Function.Arguments)
		fns.Range(func(_, value interface{}) bool {
			fn := value.(*connectedFn)
			ylog.Debug("-->", "call.Function.Name", call.Function.Name, "fns.fd.Name", fn.fd.Name)
			if call.Function.Name == fn.fd.Name {
				ylog.Debug("-----> add function", "name", fn.fd.Name, "tag", fn.tag)
				currentCall := call
				result.ToolCalls[fn.tag] = append(result.ToolCalls[fn.tag], &currentCall)
			}
			return true
		})
	}

	// messages
	return result, nil
}

// RegisterFunction registers the llm function
func (p *GeminiProvider) RegisterFunction(tag uint32, functionDefinition *ai.FunctionDefinition, connID uint64) error {
	// replace "-" in functionDefinition.Name to "_" as gemini does not support "-"
	functionDefinition.Name = strings.Replace(functionDefinition.Name, "-", "_", -1)

	fns.Store(connID, &connectedFn{
		connID: connID,
		tag:    tag,
		fd:     convertStandardToFunctionDeclaration(functionDefinition),
	})

	return nil
}

// UnregisterFunction unregister the llm function
func (p *GeminiProvider) UnregisterFunction(name string, connID uint64) error {
	fns.Delete(connID)
	return nil
}

// ListToolCalls lists the llm tool calls
func (p *GeminiProvider) ListToolCalls() (map[uint32]ai.ToolCall, error) {
	result := make(map[uint32]ai.ToolCall)

	tmp := make(map[uint32]*FunctionDeclaration)
	fns.Range(func(_, value any) bool {
		fn := value.(*connectedFn)
		tmp[fn.tag] = fn.fd
		result[fn.tag] = ai.ToolCall{
			Function: convertFunctionDeclarationToStandard(fn.fd),
		}
		return true
	})

	return result, nil
}

// GetOverview returns the overview of the AI functions, key is the tag, value is the function definition
func (p *GeminiProvider) GetOverview() (*ai.OverviewResponse, error) {
	isEmpty := true
	fns.Range(func(_, _ any) bool {
		isEmpty = false
		return false
	})

	result := &ai.OverviewResponse{
		Functions: make(map[uint32]*ai.FunctionDefinition),
	}

	if isEmpty {
		return result, nil
	}

	fns.Range(func(_, value any) bool {
		fn := value.(*connectedFn)
		result.Functions[fn.tag] = convertFunctionDeclarationToStandard(fn.fd)
		return true
	})

	return result, nil
}

// getApiUrl returns the gemini generateContent API url
func (p *GeminiProvider) getApiUrl() string {
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", p.APIKey)
}

// prepareRequestBody prepares the request body for the API
func (p *GeminiProvider) prepareRequestBody(userInstruction string) *RequestBody {
	body := &RequestBody{}

	// prepare contents
	body.Contents.Role = "user"
	body.Contents.Parts.Text = userInstruction

	// prepare tools
	toolCalls := make([]*FunctionDeclaration, 0)
	fns.Range(func(_, value interface{}) bool {
		fn := value.(*connectedFn)
		toolCalls = append(toolCalls, fn.fd)
		return true
	})
	body.Tools = make([]Tool, 0)
	if len(toolCalls) > 0 {
		body.Tools = append(body.Tools, Tool{
			FunctionDeclarations: toolCalls,
		})
	}

	return body
}

// NewProvider creates a new GeminiProvider
func NewProvider(apiKey string) *GeminiProvider {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	p := &GeminiProvider{
		APIKey: apiKey,
	}
	apiURL := p.getApiUrl()
	ylog.Debug("new gemini provider", "api_endpoint", apiURL)

	return p
}
