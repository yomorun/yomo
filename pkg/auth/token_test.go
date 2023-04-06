package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToken(t *testing.T) {
	auth := NewTokenAuth()

	auth.Init("mock-token")

	assert.Equal(t, "token", auth.Name())

	_, authed := auth.Authenticate("mock-token")
	assert.True(t, authed)

	_, authed = auth.Authenticate("other-token")
	assert.False(t, authed)
}
