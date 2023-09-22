package ylog

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	testdir := t.TempDir()

	var (
		output    = path.Join(testdir, "output.log")
		errOutput = path.Join(testdir, "err_output.log")
	)

	conf := Config{
		Level:       "info",
		Output:      output,
		ErrorOutput: errOutput,
		Format:      "json",
		DisableTime: true,
	}

	logger := slog.New(NewHandlerFromConfig(conf))

	logger.Debug("some debug", "hello", "yomo")
	logger.Info("some info", "hello", "yomo")

	logger.Error("read error", "err", io.EOF, "hello", "yomo")

	log, err := os.ReadFile(output)
	assert.NoError(t, err)
	assert.FileExists(t, output)

	data := make(map[string]string)
	err = json.Unmarshal(log, &data)
	assert.NoError(t, err)
	assert.Equal(t, data["msg"], "some info")
	assert.Equal(t, data["hello"], "yomo")

	errlog, err := os.ReadFile(errOutput)
	assert.NoError(t, err)
	assert.FileExists(t, errOutput)

	data = make(map[string]string)
	err = json.Unmarshal(errlog, &data)
	assert.NoError(t, err)
	assert.Equal(t, data["msg"], "read error")
	assert.Equal(t, data["err"], "EOF")
	assert.Equal(t, data["hello"], "yomo")

	os.Remove(output)
	os.Remove(errOutput)
}
