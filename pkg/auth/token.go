package auth

import (
	"github.com/yomorun/yomo/core/auth"
)

var _ auth.Credential = (*TokenCredential)(nil)

type TokenCredential struct {
	payload []byte
}

func NewTokenCredential(token string) *TokenCredential {
	return &TokenCredential{
		payload: []byte(token),
	}
}

func (c *TokenCredential) Payload() []byte {
	return c.payload
}

func (c *TokenCredential) Name() string {
	return "token"
}

var _ auth.Authentication = (*TokenAuth)(nil)

// TokenAuth token authentication (simple)
type TokenAuth struct {
	token string
}

func NewTokenAuth(token string) *TokenAuth {
	return &TokenAuth{token}
}

func (a *TokenAuth) Authenticate(payload []byte) bool {
	return a.token == string(payload)
}

func (a *TokenAuth) Name() string {
	return "token"
}
