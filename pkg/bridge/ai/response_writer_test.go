package ai

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/core/ylog"
)

func TestResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()

	w := NewResponseWriter(recorder, ylog.NewFromConfig(ylog.Config{}))

	h := w.SetStreamHeader()

	err := w.WriteStreamEvent(openai.ChatCompletionStreamResponse{
		ID: "chatcmpl-123",
	})
	assert.NoError(t, err)

	err = w.WriteStreamDone()
	assert.NoError(t, err)

	got := recorder.Body.String()

	want := `data: {"id":"chatcmpl-123","object":"","created":0,"model":"","choices":null,"system_fingerprint":""}

data: [DONE]`

	assert.Equal(t, want, got)
	assert.Equal(t, recorder.Header(), h)
}
