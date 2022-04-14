package auth

import (
	"github.com/yomorun/yomo/core/auth"
)

var _ auth.Credential = (*TokenCredential)(nil)

type TokenCredential struct {
	payload string
}

func NewTokenCredential(token string) *TokenCredential {
	return &TokenCredential{
		payload: token,
	}
}

func (c *TokenCredential) Payload() string {
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

func (a *TokenAuth) Authenticate(payload string) bool {
	return a.token == payload
}

func (a *TokenAuth) Name() string {
	return "token"
}
