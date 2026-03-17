package ai

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	openai "github.com/yomorun/go-openai"
)

func resetRecorderState() {
	recorderOnce = sync.Once{}
	recorderInstance = nil
}

func TestRecordWriteJSONL(t *testing.T) {
	resetRecorderState()

	tmpDir := t.TempDir()
	recordPath := filepath.Join(tmpDir, "records.jsonl")

	prev := os.Getenv(recordPathEnv)
	if err := os.Setenv(recordPathEnv, recordPath); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if prev == "" {
			_ = os.Unsetenv(recordPathEnv)
			return
		}
		_ = os.Setenv(recordPathEnv, prev)
	}()

	recorder := getResponseRecorder()
	if recorder == nil {
		t.Fatal("expected recorder to be enabled")
	}

	entry := recordEntry{
		Time:      "2026-03-15T12:00:00Z",
		TransID:   "test-trans",
		CallIndex: 1,
		Provider:  "mock-provider",
		Stream:    false,
		Request: openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: "hi"},
			},
		},
		Response: &openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "ok"}},
			},
		},
	}

	err := recorder.Record(entry)
	assert.NoError(t, err)

	f, err := os.Open(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	reader := bufio.NewScanner(f)
	if !reader.Scan() {
		t.Fatal("expected a record line")
	}

	var got recordEntry
	decodeErr := json.Unmarshal(reader.Bytes(), &got)
	assert.NoError(t, decodeErr)
	assert.Equal(t, entry.TransID, got.TransID)
	assert.Equal(t, entry.CallIndex, got.CallIndex)
	assert.Equal(t, entry.Provider, got.Provider)
	assert.Equal(t, entry.Stream, got.Stream)
	assert.Equal(t, entry.Request.Model, got.Request.Model)
	assert.Equal(t, entry.Response.Choices[0].Message.Content, got.Response.Choices[0].Message.Content)
}

func TestRecordDisabledWithoutEnv(t *testing.T) {
	resetRecorderState()
	prev := os.Getenv(recordPathEnv)
	if err := os.Unsetenv(recordPathEnv); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if prev == "" {
			return
		}
		_ = os.Setenv(recordPathEnv, prev)
	}()

	recorder := getResponseRecorder()
	assert.Nil(t, recorder)
}
