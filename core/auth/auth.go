package auth

import (
	"github.com/yomorun/yomo/core/frame"
)

type AuthType byte

const (
	AuthTypeNone       AuthType = 0x0
	AuthTypeAppKey     AuthType = 0x1
	AuthTypePublicKey  AuthType = 0x2
	AuthTypePrivateKey AuthType = 0x3
)

func (a AuthType) String() string {
	switch a {
	case AuthTypeAppKey:
		return "AppKey"
	case PublicKey:
		return "PublicKey"
	case PrivateKey:
		return "PrivateKey"
	default:
		return "None"
	}
}

// Authentication for server
type Authentication interface {
	Type() AuthType
	Authenticate(f *frame.HandshakeFrame) bool
}

// Credential for client
type Credential interface {
	AppID() string
	Type() AuthType
	Payload() []byte
}

// None auth

var _ Authentication = (*AuthNone)(nil)

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

func (c *CredentialNone) AppID() string {
	return ""
}

func (c *CredentialNone) Type() AuthType {
	return AuthTypeNone
}

func (c *CredentialNone) Payload() []byte {
	return nil
}
