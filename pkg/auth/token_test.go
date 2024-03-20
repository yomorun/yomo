package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToken(t *testing.T) {
	auth := NewTokenAuth()

	auth.Init("mock-token")

	assert.Equal(t, "token", auth.Name())

	_, err := auth.Authenticate("mock-token")
	assert.NoError(t, err)

	_, err = auth.Authenticate("other-token")
	assert.EqualError(t, err, "invalid token: other-token")
}
