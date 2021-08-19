package zipper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfig(t *testing.T) {
	// server
	assert.Equal(t, "Server", testConfig.Name)
	assert.Equal(t, "127.0.0.1", testConfig.Host)
	assert.Equal(t, 8211, testConfig.Port)
	// functions
	assert.Equal(t, 3, len(testConfig.Functions))
	for i, fn := range testConfig.Functions {
		i++
		expected := fmt.Sprintf("func%d", i)
		assert.Equal(t, expected, fn.Name)
	}
}
