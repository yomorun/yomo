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
	"os"
	"strings"
	"text/template"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
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
	Response        string `json:"response"`
	Done            bool   `json:"done"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
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

	mistralTmpl = "{{ if .Tools }}[AVAILABLE_TOOLS] {{.Tools}} [/AVAILABLE_TOOLS] {{ end }}[INST] {{ if .System }}{{ .System }} {{ end }}{{ .Prompt }} [/INST]"
)

func makeOllamaRequestBody(req openai.ChatCompletionRequest) (io.Reader, error) {
	if req.Model == "" {
		req.Model = "mistral"
	}

	if !strings.HasPrefix(req.Model, "mistral") {
		return nil, errors.New("currently only Mistral models are supported, see https://ollama.com/library/mistral")
	}

	t := &templateRequest{
		System: defaultSystem,
		Tools:  "[]",
	}
	for _, msg := range req.Messages {
		switch strings.ToLower(msg.Role) {
		case openai.ChatMessageRoleSystem:
			t.System = msg.Content
		case openai.ChatMessageRoleUser:
			t.Prompt += msg.Content + " "
		case openai.ChatMessageRoleTool:
			t.Prompt += msg.Content + " "
		}
	}

	if len(req.Tools) > 0 {
		t.System += systemToolExtra

		req.Stream = false

		tools, err := json.Marshal(req.Tools)
		if err != nil {
			return nil, err
		}
		t.Tools = string(tools)
	}

	ylog.Debug("ollama chat request", "model", req.Model, "system", t.System, "prompt", t.Prompt, "tools", t.Tools)

	tmpl, err := template.New("ollama").Parse(mistralTmpl)
	if err != nil {
		return nil, err
	}

	prompt := bytes.NewBufferString("")
	err = tmpl.Execute(prompt, t)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(&ollamaRequest{
		Model:  req.Model,
		Prompt: prompt.String(),
		Raw:    true,
		Stream: req.Stream,
	})
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(body), nil
}

func parseToolCallsFromResponse(response string) []openai.ToolCall {
	toolCalls := make([]openai.ToolCall, 0)

	response = strings.TrimPrefix(response, "[TOOL_CALLS]")
	for _, v := range strings.Split(response, "\n") {
		var functions []mistralFunction
		if json.Unmarshal([]byte(v), &functions) == nil {
			for _, f := range functions {
				arguments, _ := json.Marshal(f.Arguments)
				toolCalls = append(toolCalls, openai.ToolCall{
					ID:   id.New(),
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      f.Name,
						Arguments: string(arguments),
					},
				})
			}
		}
	}

	return toolCalls
}

// GetChatCompletions implements ai.LLMProvider.
func (p *Provider) GetChatCompletions(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	res := openai.ChatCompletionResponse{
		ID:      "chatcmpl-" + id.New(29),
		Model:   req.Model,
		Created: time.Now().Unix(),
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: "error occured during inference period",
				},
				FinishReason: openai.FinishReasonStop,
			},
		},
	}

	urlPath, err := url.JoinPath(p.Endpoint, "api/generate")
	if err != nil {
		return res, err
	}

	body, err := makeOllamaRequestBody(req)
	if err != nil {
		return res, err
	}

	client := http.Client{}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPath, body)
	if err != nil {
		return res, err
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(request)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("ollama inference error: %s", resp.Status)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return res, err
	}

	var o ollamaResponse
	err = json.Unmarshal(buf, &o)
	if err != nil {
		return res, err
	}

	ylog.Debug("ollama chat response", "response", o.Response)
	ylog.Debug("ollama chat usage", "prompt_tokens", o.PromptEvalCount, "completion_tokens", o.EvalCount)

	if o.Response != "" {
		res.Choices[0].Message.Content = o.Response

		if len(req.Tools) > 0 {
			toolCalls := parseToolCallsFromResponse(o.Response)
			if len(toolCalls) > 0 {
				res.Choices[0].FinishReason = openai.FinishReasonToolCalls
				res.Choices[0].Message.ToolCalls = toolCalls
			}
		}

		res.Usage = openai.Usage{
			PromptTokens:     o.PromptEvalCount,
			CompletionTokens: o.EvalCount,
			TotalTokens:      o.PromptEvalCount + o.EvalCount,
		}
	}

	return res, nil
}

type streamResponse struct {
	reader    io.ReadCloser
	withTools bool
	index     int
	res       openai.ChatCompletionStreamResponse
}

func (s *streamResponse) Recv() (openai.ChatCompletionStreamResponse, error) {
	var buf []byte
	var err error

	if s.withTools {
		buf, err = io.ReadAll(s.reader)
		if err != nil {
			return s.res, err
		}
	} else {
		buf = make([]byte, 1024)
		n, err := s.reader.Read(buf)
		if err != nil {
			return s.res, err
		}
		buf = buf[:n]
	}

	ylog.Debug("ollama chat stream", "delta", string(buf))
	if len(buf) == 0 {
		s.reader.Close()
		return s.res, io.EOF
	}

	var o ollamaResponse
	err = json.Unmarshal(buf, &o)
	if err != nil {
		return s.res, err
	}

	ylog.Debug("ollama chat stream response", "response", o.Response, "done", o.Done)

	s.res.Choices[0].Index++
	s.res.Choices[0].Delta.Content = o.Response
	if o.Done {
		ylog.Debug("ollama chat stream usage", "prompt_tokens", o.PromptEvalCount, "completion_tokens", o.EvalCount)

		s.res.Choices[0].FinishReason = openai.FinishReasonStop
		s.res.Usage = &openai.Usage{
			PromptTokens:     o.PromptEvalCount,
			CompletionTokens: o.EvalCount,
			TotalTokens:      o.PromptEvalCount + o.EvalCount,
		}

		if s.withTools {
			toolCalls := parseToolCallsFromResponse(o.Response)
			if len(toolCalls) > 0 {
				for i := 0; i < len(toolCalls); i++ {
					index := i
					toolCalls[index].Index = &index
				}
				s.res.Choices[0].FinishReason = openai.FinishReasonToolCalls
				s.res.Choices[0].Delta.ToolCalls = toolCalls
			}
		}
	}

	return s.res, nil
}

// GetChatCompletionsStream implements ai.LLMProvider.
func (p *Provider) GetChatCompletionsStream(ctx context.Context, req openai.ChatCompletionRequest) (provider.ResponseRecver, error) {
	urlPath, err := url.JoinPath(p.Endpoint, "api/generate")
	if err != nil {
		return nil, err
	}

	body, err := makeOllamaRequestBody(req)
	if err != nil {
		return nil, err
	}

	client := http.Client{}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPath, body)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama inference error: %s", resp.Status)
	}

	return &streamResponse{
		reader:    resp.Body,
		withTools: len(req.Tools) > 0,
		index:     0,
		res: openai.ChatCompletionStreamResponse{
			ID:      "chatcmpl-" + id.New(29),
			Model:   req.Model,
			Created: time.Now().Unix(),
			Choices: []openai.ChatCompletionStreamChoice{
				{
					Index: -1,
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Role: openai.ChatMessageRoleAssistant,
					},
				},
			},
		},
	}, nil
}

// Name implements ai.LLMProvider.
func (p *Provider) Name() string {
	return "ollama"
}

// NewProvider creates a new OllamaProvider
func NewProvider(endpoint string) *Provider {
	if endpoint == "" {
		v, ok := os.LookupEnv("OLLAMA_API_ENDPOINT")
		if ok {
			endpoint = v
		} else {
			endpoint = "http://localhost:11434/"
		}
	}
	return &Provider{endpoint}
}
