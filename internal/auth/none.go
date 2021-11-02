package auth

import "github.com/yomorun/yomo/internal/frame"

var _ = Authentication(&AuthNone{})

type AuthNone struct{}

func NewAuthNone() *AuthNone {
	return &AuthNone{}
}

func (a *AuthNone) Type() AuthType {
	return AuthTypeNone
}

func (a *AuthNone) Authenticate(f *frame.HandshakeFrame) bool {
	return true
}

var _ = Credential(&CredentialNone{})

type CredentialNone struct{}

func NewCredendialNone() *CredentialNone {
	return &CredentialNone{}
}

func (c *CredentialNone) Type() AuthType {
	return AuthTypeNone
}

func (c *CredentialNone) Payload() []byte {
	return nil
}
