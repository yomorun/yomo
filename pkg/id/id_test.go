package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	str := New()
	assert.IsType(t, "", str)

	tid := TID()
	assert.IsType(t, "", tid)
	assert.Equal(t, 32, len(tid))

	sid := SID()
	assert.IsType(t, "", sid)
	assert.Equal(t, 16, len(sid))
}
