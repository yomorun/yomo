// Package oai provides the ability to call OpenAI api
package oai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
)

type OpenAIClient struct{}

var _ OpenAIRequester = &OpenAIClient{}

// ChatCompletions is the function to call OpenAI api
func (c *OpenAIClient) ChatCompletions(
	ctx context.Context,
	apiEndpoint string,
	authHeaderKey string,
	authHeaderValue string,
	chatCompletionRequest *ai.ChatCompletionRequest,
) (*ai.ChatCompletionResponse, error) {
	jsonBody, err := json.Marshal(chatCompletionRequest)
	if err != nil {
		return nil, err
	}
	ylog.Debug("> chat completions request", "body", string(jsonBody))
	// create request
	req, err := http.NewRequestWithContext(ctx, "POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authHeaderKey, authHeaderValue)
	// send request
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
	ylog.Debug("< chat completions response", "body", string(respBody))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("chat completions response status code is %d", resp.StatusCode)
	}

	// handle response
	var result ai.ChatCompletionResponse
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		return nil, err
	}
	return &result, err
}
