package ylog

import (
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
		DisableTime: true,
	}

	logger := slog.New(NewHandlerFromConfig(conf))

	logger.Debug("some debug", "hello", "yomo")
	logger.Info("some info", "hello", "yomo")
	logger.Warn("some waring", "hello", "yomo")

	logger.Error("error", "err", io.EOF, "hello", "yomo")

	log, err := os.ReadFile(output)

	assert.NoError(t, err)
	assert.FileExists(t, output)
	assert.Equal(t, "\x1b[92mINF\x1b[0m some info \x1b[2mhello=\x1b[0myomo\n\x1b[93mWRN\x1b[0m some waring \x1b[2mhello=\x1b[0myomo\n", string(log))

	errlog, err := os.ReadFile(errOutput)

	assert.NoError(t, err)
	assert.FileExists(t, errOutput)
	assert.Equal(t, "\x1b[91mERR\x1b[0m error \x1b[2merr=\x1b[0mEOF \x1b[2mhello=\x1b[0myomo\n", string(errlog))

	os.Remove(output)
	os.Remove(errOutput)
}
