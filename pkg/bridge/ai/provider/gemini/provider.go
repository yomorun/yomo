// Package gemini provides the Gemini AI provider
package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	bridgeai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// Provider is the provider for Gemini
type Provider struct {
	APIKey string
}

// NewProvider creates a new GeminiProvider
func NewProvider(apiKey string) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	p := &Provider{
		APIKey: apiKey,
	}
	apiURL := p.getAPIURL()
	ylog.Debug("new gemini provider", "api_endpoint", apiURL)

	return p
}

var _ bridgeai.LLMProvider = &Provider{}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return "gemini"
}

// GetChatCompletions get chat completions for ai service
func (p *Provider) GetChatCompletions(chatCompletionRequest *ai.ChatCompletionRequest) (*ai.ChatCompletionResponse, error) {
	// request API
	jsonBody, err := json.Marshal(convertStandardToRequest(chatCompletionRequest))
	if err != nil {
		ylog.Error(err.Error())
		return nil, err
	}

	ylog.Debug("gemini api request", "body", string(jsonBody))

	req, err := http.NewRequest("POST", p.getAPIURL(), bytes.NewBuffer(jsonBody))
	if err != nil {
		ylog.Error(err.Error())
		// fmt.Println("Error creating new request:", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ylog.Error(err.Error())
		// fmt.Println("Error making request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		ylog.Error(err.Error())
		// fmt.Println("Error reading response body:", err)
		return nil, err
	}
	ylog.Debug("gemini api response", "status", resp.StatusCode, "body", string(respBody))

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini provider api response status code is %d", resp.StatusCode)
	}

	// parse response
	response, err := parseAPIResponseBody(respBody)
	if err != nil {
		ylog.Error(err.Error())
		return nil, err
	}

	result := convertResponseToStandard(response)
	ylog.Debug("gemini chat completion", "response", response, "result", result)
	return result, nil
}

// getAPIURL returns the gemini generateContent API url
func (p *Provider) getAPIURL() string {
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", p.APIKey)
}
