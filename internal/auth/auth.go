package auth

import "github.com/yomorun/yomo/internal/frame"

type AuthType byte

const (
	AuthTypeNone   AuthType = 0x0
	AuthTypeAppKey AuthType = 0x1
)

func (a AuthType) String() string {
	switch a {
	case AuthTypeAppKey:
		return "AppKey"
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
	Type() AuthType
	Payload() []byte
}
