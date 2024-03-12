// Package cfazure is used to provide the Azure OpenAI service
package cfazure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	// automatically load .env file
	_ "github.com/joho/godotenv/autoload"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/azopenai"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

// CloudflareAzureProvider is the provider for Azure OpenAI
type CloudflareAzureProvider struct {
	APIKey       string
	Resource     string
	DeploymentID string
	APIVersion   string
	CfEndpoint   string
}

// NewProvider creates a new AzureOpenAIProvider
func NewProvider(cfEndpoint string, apiKey string, resource string, deploymentID string, apiVersion string) *CloudflareAzureProvider {
	if cfEndpoint == "" || apiKey == "" || resource == "" || deploymentID == "" || apiVersion == "" {
		ylog.Error("parameters are required", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID, "apiVersion", apiVersion)
		os.Exit(-1)
		return nil
	}
	ylog.Debug("CloudflareAzureProvider", "cfEndpoint", cfEndpoint, "apiKey", apiKey, "resource", resource, "deploymentID", deploymentID, "apiVersion", apiVersion)
	return &CloudflareAzureProvider{
		CfEndpoint:   cfEndpoint,   // https://gateway.ai.cloudflare.com/v1/111111111111111111/ai-cc-test
		APIKey:       apiKey,       // azure api key
		Resource:     resource,     // azure resource
		DeploymentID: deploymentID, // azure deployment id
		APIVersion:   apiVersion,   // azure api version
	}
}

// Name returns the name of the provider
func (p *CloudflareAzureProvider) Name() string {
	return "cloudflare_azure"
}

// GetChatCompletions get chat completions for ai service
func (p *CloudflareAzureProvider) GetChatCompletions(userInstruction string, md metadata.M) (*ai.InvokeResponse, error) {
	tcs, err := register.ListToolCalls(md)
	if err != nil {
		return nil, err
	}

	if len(tcs) == 0 {
		ylog.Error("tools is empty")
		return &ai.InvokeResponse{Content: "no toolcalls"}, ai.ErrNoFunctionCall
	}

	// messages
	messages := []azopenai.ReqMessage{
		{Role: "system", Content: `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous. If you don't know the answer, stop the conversation by saying "no func call".`},
		{Role: "user", Content: userInstruction},
	}

	// prepare tools
	toolCalls := make([]ai.ToolCall, len(tcs))
	idx := 0
	for _, tc := range tcs {
		toolCalls[idx] = tc
		idx++
	}

	body := azopenai.ReqBody{Messages: messages, Tools: toolCalls}

	ylog.Debug("request", "tools", len(toolCalls), "messages", messages)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/azure-openai/%s/%s/chat/completions?api-version=%s", p.CfEndpoint, p.Resource, p.DeploymentID, p.APIVersion)
	ylog.Debug("chat completions request", "url", url)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", p.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ylog.Debug("response", "body", respBody)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ai response status code is %d", resp.StatusCode)
	}

	var respBodyStruct azopenai.RespBody
	err = json.Unmarshal(respBody, &respBodyStruct)
	if err != nil {
		return nil, err
	}

	choice := respBodyStruct.Choices[0]
	ylog.Debug(">>finish_reason", "reason", choice.FinishReason)

	calls := respBodyStruct.Choices[0].Message.ToolCalls
	content := respBodyStruct.Choices[0].Message.Content

	ylog.Debug("--response calls", "calls", calls)

	result := &ai.InvokeResponse{}
	if len(calls) == 0 {
		result.Content = content
		return result, ai.ErrNoFunctionCall
	}

	// functions may be more than one
	for _, call := range calls {
		for tag, tc := range tcs {
			if tc.Equal(&call) {
				// Use toolCalls because tool_id is required in the following llm request
				if result.ToolCalls == nil {
					result.ToolCalls = make(map[uint32][]*ai.ToolCall)
				}

				currentCall := call
				result.ToolCalls[tag] = append(result.ToolCalls[tag], &currentCall)
			}
		}
	}

	// sfn maybe disconnected, so we need to check if there is any function call
	if len(result.ToolCalls) == 0 {
		return nil, ai.ErrNoFunctionCall
	}
	return result, nil
}
