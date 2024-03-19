// Package auth provides token based authentication
package auth

import (
	"fmt"

	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/metadata"
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
func (a *TokenAuth) Authenticate(payload string) (metadata.M, error) {
	if a.token == payload {
		return metadata.M{}, nil
	}
	return metadata.M{}, fmt.Errorf("invalid token: %s", payload)
}

// Name authentication name
func (a *TokenAuth) Name() string {
	return "token"
}

func init() {
	auth.Register(NewTokenAuth())
}
