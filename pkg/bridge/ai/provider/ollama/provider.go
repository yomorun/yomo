package ollama

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/id"
)

// Provider is the provider for Ollama
type Provider struct {
	Endpoint string
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Raw    bool   `json:"raw"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

type templateRequest struct {
	Tools  string
	System string
	Prompt string
}

type mistralFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

const (
	defaultSystem = "You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task."

	systemToolExtra = "If the question of the user matched the description of a tool, the tool will be called, and only the function description JSON object should be returned. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous."

	mistralTmpl = "[AVAILABLE_TOOLS] {{.Tools}} [/AVAILABLE_TOOLS][INST] {{ if .System }}{{ .System }} {{ end }}{{ .Prompt }} [/INST]"
)

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (openai.ChatCompletionResponse, error) {
	res := openai.ChatCompletionResponse{
		ID:      "chatcmpl-" + id.New(29),
		Model:   req.Model,
		Created: time.Now().Unix(),
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Role:    "assitant",
					Content: "error occured during inference period",
				},
				FinishReason: "stop",
			},
		},
		Usage: openai.Usage{}, // todo
	}

	if !strings.HasPrefix(req.Model, "mistral") {
		return res, errors.New("currently only Mistral models are supported, see https://ollama.com/library/mistral")
	}

	t := &templateRequest{
		System: defaultSystem,
		Tools:  "[]",
	}
	for _, msg := range req.Messages {
		switch strings.ToLower(msg.Role) {
		case "system":
			t.System = msg.Content
		case "user":
			t.Prompt += msg.Content
		case "tool":
			t.Prompt += msg.Content
		}
	}

	if req.Tools != nil {
		t.System += systemToolExtra

		tools, err := json.Marshal(req.Tools)
		if err != nil {
			return res, err
		}
		t.Tools = string(tools)
	}

	ylog.Debug("ollama chat request", "model", req.Model, "system", t.System, "prompt", t.Prompt, "tools", t.Tools)

	tmpl, err := template.New("ollama").Parse(mistralTmpl)
	if err != nil {
		return res, err
	}

	prompt := bytes.NewBufferString("")
	err = tmpl.Execute(prompt, t)
	if err != nil {
		return res, err
	}

	body, err := json.Marshal(&ollamaRequest{
		Model:  req.Model,
		Prompt: prompt.String(),
		Raw:    true,
		Stream: req.Stream,
	})
	if err != nil {
		return res, err
	}

	urlPath, err := url.JoinPath(p.Endpoint, "api/generate")
	if err != nil {
		return res, err
	}

	resp, err := http.Post(urlPath, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return res, err
	}

	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("ollama inference error: %s", resp.Status)
	}

	ylog.Debug("ollama chat response", "body", string(body))

	var o ollamaResponse
	err = json.Unmarshal(body, &o)
	if err != nil {
		return res, err
	}

	if o.Response != "" {
		o.Response = strings.TrimPrefix(o.Response, "[TOOL_CALLS]")

		res.Choices[0].Message.Content = o.Response

		var functions []mistralFunction
		err = json.Unmarshal([]byte(o.Response), &functions)
		if err == nil {
			res.Choices[0].FinishReason = "tool_calls"
			res.Choices[0].Message.ToolCalls = make([]openai.ToolCall, 0)
			for _, f := range functions {
				arguments, _ := json.Marshal(f.Arguments)
				res.Choices[0].Message.ToolCalls = append(
					res.Choices[0].Message.ToolCalls, openai.ToolCall{
						ID:   id.New(),
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      f.Name,
							Arguments: string(arguments),
						},
					},
				)
			}
		}
	}

	return res, nil
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest, _ metadata.M) (ai.ResponseRecver, error) {
	panic("unimplemented")
}

// Name implements ai.LLMProvider.
func (p *Provider) Name() string {
	return "ollama"
}

// NewProvider creates a new OllamaProvider
func NewProvider(endpoint string) *Provider {
	return &Provider{endpoint}
}
