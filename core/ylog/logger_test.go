package ylog

import (
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slog"
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

	logger.Error("error", io.EOF, "hello", "yomo")

	log, err := os.ReadFile(output)

	assert.NoError(t, err)
	assert.FileExists(t, output)
	assert.Equal(t, "level=INFO msg=\"some info\" hello=yomo\nlevel=WARN msg=\"some waring\" hello=yomo\n", string(log))

	errlog, err := os.ReadFile(errOutput)

	assert.NoError(t, err)
	assert.FileExists(t, errOutput)
	assert.Equal(t, "level=ERROR msg=error err=EOF hello=yomo\n", string(errlog))

	os.Remove(output)
	os.Remove(errOutput)
}
