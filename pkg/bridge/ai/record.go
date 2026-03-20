package ai

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	openai "github.com/yomorun/go-openai"
)

const recordPathEnv = "YOMO_PROVIDER_RECORD_PATH"

type responseRecorder struct {
	path string
	mu   sync.Mutex
}

type recordEntry struct {
	Time         string                                `json:"ts"`
	TransID      string                                `json:"trans_id,omitempty"`
	CallIndex    int                                   `json:"call_index"`
	Provider     string                                `json:"provider,omitempty"`
	Stream       bool                                  `json:"stream"`
	Request      openai.ChatCompletionRequest          `json:"request"`
	Response     *openai.ChatCompletionResponse        `json:"response,omitempty"`
	StreamChunks []openai.ChatCompletionStreamResponse `json:"stream_chunks,omitempty"`
}

var recorderOnce sync.Once
var recorderInstance *responseRecorder

func getResponseRecorder() *responseRecorder {
	recorderOnce.Do(func() {
		path := os.Getenv(recordPathEnv)
		if path == "" {
			return
		}
		recorderInstance = &responseRecorder{path: path}
	})
	return recorderInstance
}

func (r *responseRecorder) Record(entry recordEntry) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	return enc.Encode(entry)
}

func nowISO8601() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func cloneChatCompletionRequest(req openai.ChatCompletionRequest) openai.ChatCompletionRequest {
	cloned := req
	if req.Messages != nil {
		cloned.Messages = append([]openai.ChatCompletionMessage(nil), req.Messages...)
	}
	if req.Tools != nil {
		cloned.Tools = append([]openai.Tool(nil), req.Tools...)
	}
	return cloned
}
