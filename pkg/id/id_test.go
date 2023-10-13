package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	t.Run("new a random id", func(t *testing.T) {
		str := New()
		assert.IsType(t, "", str)
	})

	t.Run("new trace id", func(t *testing.T) {
		traceID := NewTraceID()
		assert.IsType(t, "", traceID)
		assert.Equal(t, 32, len(traceID))
	})

	t.Run("new span id", func(t *testing.T) {
		spanID := NewSpanID()
		assert.IsType(t, "", spanID)
		assert.Equal(t, 16, len(spanID))
	})
}
