package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

// GeminiProvider is the provider for Gemini
type GeminiProvider struct {
	APIKey string
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

var _ bridgeai.LLMProvider = &GeminiProvider{}

// Name returns the name of the provider
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// GetChatCompletions get chat completions for ai service
func (p *GeminiProvider) GetChatCompletions(userInstruction string, baseSystemMessage string, chainMessage ai.ChainMessage, md metadata.M, withTool bool) (*ai.InvokeResponse, error) {
	if !withTool {
		ylog.Warn("Gemini call should have tool calls")
	}

	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return nil, err
	}
	if len(tcs) == 0 {
		ylog.Error(ai.ErrNoFunctionCall.Error())
		return &ai.InvokeResponse{Content: "no toolcalls"}, ai.ErrNoFunctionCall
	}

	// prepare request body
	body, fds := p.prepareRequest(userInstruction, tcs)

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
	for tag, tc := range tcs {
		for _, fd := range fds {
			if fd.Name == tc.Function.Name {
				ylog.Debug("-----> add function", "name", fd.Name, "tag", tag)
				currentCall := tc
				result.ToolCalls[tag] = append(result.ToolCalls[tag], &currentCall)
			}
		}
	}

	// messages
	return result, nil
}

// getApiUrl returns the gemini generateContent API url
func (p *GeminiProvider) getApiUrl() string {
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", p.APIKey)
}

// prepareRequestBody prepares the request body for the API
func (p *GeminiProvider) prepareRequest(userInstruction string, tcs map[uint32]ai.ToolCall) (*RequestBody, []*FunctionDeclaration) {
	body := &RequestBody{}

	// prepare contents
	body.Contents.Role = "user"
	body.Contents.Parts.Text = userInstruction

	// prepare tools
	toolCalls := make([]*FunctionDeclaration, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = convertStandardToFunctionDeclaration(tc.Function)
		idx++
	}
	body.Tools = make([]Tool, 0)
	if len(toolCalls) > 0 {
		body.Tools = append(body.Tools, Tool{
			FunctionDeclarations: toolCalls,
		})
	}

	return body, toolCalls
}
