package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestZipperRun(t *testing.T) {
	zipper := NewZipperWithOptions("zipper", "localhost:9001")
	time.Sleep(time.Second)
	assert.NotNil(t, zipper)
	err := zipper.Close()
	time.Sleep(time.Second)
	assert.Nil(t, err)
}
