package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLog(t *testing.T) {
	assert.Equal(t, Level(88).String(), "")
	assert.Equal(t, DebugLevel.String(), "DEBUG")
	assert.Equal(t, WarnLevel.String(), "WARN")
	assert.Equal(t, ErrorLevel.String(), "ERROR")
	assert.Equal(t, InfoLevel.String(), "INFO")
}
