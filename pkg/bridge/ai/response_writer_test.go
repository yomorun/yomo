package ai

import (
	"net/http/httptest"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()

	w := NewResponseWriter(recorder)

	h := w.SetStreamHeader()

	err := w.WriteStreamEvent(openai.ChatCompletionResponse{
		ID: "chatcmpl-123",
	})
	assert.NoError(t, err)

	err = w.WriteStreamDone()
	assert.NoError(t, err)

	got := recorder.Body.String()

	want := `data: {"id":"chatcmpl-123","object":"","created":0,"model":"","choices":null,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"prompt_tokens_details":null,"completion_tokens_details":null},"system_fingerprint":""}

data: [DONE]`

	assert.Equal(t, want, got)
	assert.Equal(t, recorder.Header(), h)
}
