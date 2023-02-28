// Package auth provides token based authentication
package auth

import (
	"github.com/yomorun/yomo/core/auth"
)

var _ auth.Authentication = (*TokenAuth)(nil)

// TokenAuth token authentication (simple)
type TokenAuth struct {
	token string
}

// NewTokenAuth create a token authentication
func NewTokenAuth() *TokenAuth {
	return &TokenAuth{}
}

// Init authentication initialize arguments
func (a *TokenAuth) Init(args ...string) {
	if len(args) > 0 {
		a.token = args[0]
	}
}

// Authenticate authentication client's credential
func (a *TokenAuth) Authenticate(payload string) bool {
	return a.token == payload
}

// Name authentication name
func (a *TokenAuth) Name() string {
	return "token"
}

func init() {
	auth.Register(NewTokenAuth())
}
