package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestZipperRun(t *testing.T) {
	zipper, err := NewZipper("zipper", nil, nil)
	assert.Nil(t, err)
	time.Sleep(time.Second)
	assert.NotNil(t, zipper)
	err = zipper.Close()
	time.Sleep(time.Second)
	assert.Nil(t, err)
}
