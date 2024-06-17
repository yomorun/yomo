package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/sashabaranov/go-openai"
)

type Mock struct {
	name       string
	resp       openai.ChatCompletionResponse
	streamResp *ChatCompletionStreamResponse
}

type ChatCompletionStreamResponse struct {
	mu    sync.Mutex
	items []openai.ChatCompletionStreamResponse
}

func NewMock(name string, data ...mockData) (LLMProvider, error) {
	p := &Mock{
		name: name,
	}
	if len(data) == 0 {
		return p, nil
	}

	for _, d := range data {
		if err := d.apply(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (m *ChatCompletionStreamResponse) Recv() (openai.ChatCompletionStreamResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.items) == 0 {
		return openai.ChatCompletionStreamResponse{}, io.EOF
	}
	item := m.items[0]
	m.items = m.items[1:]
	return item, nil
}

type mockData interface {
	apply(*Mock) error
}

type applyFunc func(*Mock) error

func (f applyFunc) apply(mp *Mock) error { return f(mp) }

func MockChatCompletionResponse(str string) mockData {
	return applyFunc(func(m *Mock) error {
		return json.Unmarshal([]byte(str), &m.resp)
	})
}

func MockChatCompletionStreamResponse(str string) mockData {
	scanner := bufio.NewScanner(strings.NewReader(str))
	scanner.Split(bufio.ScanLines)

	var (
		err        error
		streamResp = new(ChatCompletionStreamResponse)
	)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "data: ") {
			jsonStr := text[6:]
			if jsonStr == "[DONE]" {
				break
			}
			var item openai.ChatCompletionStreamResponse
			if err = json.Unmarshal([]byte(jsonStr), &item); err != nil {
				err = fmt.Errorf("json.Unmarshal: %w", err)
				return applyFunc(func(m *Mock) error {
					return err
				})
			}
			streamResp.mu.Lock()
			streamResp.items = append(streamResp.items, item)
			streamResp.mu.Unlock()
		}
	}
	return applyFunc(func(m *Mock) error {
		m.streamResp = streamResp
		return nil
	})
}

func (m *Mock) GetChatCompletions(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return m.resp, nil
}

func (m *Mock) GetChatCompletionsStream(_ context.Context, req openai.ChatCompletionRequest) (ResponseRecver, error) {
	return m.streamResp, nil
}

func (m *Mock) Name() string {
	return m.name
}
