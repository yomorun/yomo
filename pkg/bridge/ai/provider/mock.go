// Package provider defines the ai.Provider interface and provides a mock provider for unittest.
package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// Mock implements the ai.Provider interface.
// And it can be used for recording requests and mocking responses.
type Mock struct {
	name string

	reqs []openai.ChatCompletionRequest

	// calling function once will return and remove one element from resp and streamResp.
	resp       []openai.ChatCompletionResponse
	streamResp []*ChatCompletionStreamResponse
}

// ChatCompletionStreamResponse has Recv() function so it implements the ResponseRecver interface.
type ChatCompletionStreamResponse struct {
	items []openai.ChatCompletionStreamResponse
}

// NewMock returns a mock provider.
func NewMock(name string, data ...MockData) (*Mock, error) {
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

// Recv implements the ResponseRecver interface.
func (m *ChatCompletionStreamResponse) Recv() (openai.ChatCompletionStreamResponse, error) {
	if len(m.items) == 0 {
		return openai.ChatCompletionStreamResponse{}, io.EOF
	}
	item := m.items[0]
	m.items = m.items[1:]
	return item, nil
}

// MockData supplys mock response data to the mock provider.
type MockData interface {
	apply(*Mock) error
}

type applyFunc func(*Mock) error

func (f applyFunc) apply(mp *Mock) error { return f(mp) }

// MockChatCompletionResponse supplys mock response data to the mock provider.
func MockChatCompletionResponse(str ...string) MockData {
	return applyFunc(func(m *Mock) error {
		m.resp = make([]openai.ChatCompletionResponse, len(str))
		for i, s := range str {
			if err := json.Unmarshal([]byte(s), &m.resp[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// MockChatCompletionStreamResponse supplys mock response data in form of stream to the mock provider.
func MockChatCompletionStreamResponse(str ...string) MockData {
	streamRespArr := make([]*ChatCompletionStreamResponse, len(str))
	for i, s := range str {
		scanner := bufio.NewScanner(strings.NewReader(s))
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
					return applyFunc(func(m *Mock) error {
						return err
					})
				}
				streamResp.items = append(streamResp.items, item)
			}
		}
		streamRespArr[i] = streamResp
	}

	return applyFunc(func(m *Mock) error {
		m.streamResp = streamRespArr
		return nil
	})
}

// GetChatCompletions implements the ai.Provider interface.
func (m *Mock) GetChatCompletions(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	data, _ := json.Marshal(&req)
	fmt.Println("[mock provider] request:", string(data))

	m.reqs = append(m.reqs, req)

	item := m.resp[0]
	m.resp = m.resp[1:]
	return item, nil
}

// GetChatCompletionsStream implements the ai.Provider interface.
func (m *Mock) GetChatCompletionsStream(_ context.Context, req openai.ChatCompletionRequest) (ResponseRecver, error) {
	data, _ := json.Marshal(&req)
	fmt.Println("[mock provider] stream request:", string(data))

	m.reqs = append(m.reqs, req)

	item := m.streamResp[0]
	m.streamResp = m.streamResp[1:]
	return item, nil
}

// RequestRecords returns the request records.
func (m *Mock) RequestRecords() []openai.ChatCompletionRequest {
	return m.reqs
}

// Name returns the provider name.
func (m *Mock) Name() string {
	return m.name
}
