// Package auth provides authentication.
package auth

import (
	"errors"
	"strings"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

var (
	auths = make(map[string]Authentication)
)

// Authentication for server
type Authentication interface {
	// Init authentication initialize arguments
	Init(args ...string)
	// Authenticate authentication client's credential
	Authenticate(payload string) (metadata.M, error)
	// Name authentication name
	Name() string
}

// Register register authentication
func Register(authentication Authentication) {
	auths[authentication.Name()] = authentication
}

// GetAuth get authentication by name
func GetAuth(name string) (Authentication, bool) {
	auth, ok := auths[name]
	return auth, ok
}

// Credential client credential
type Credential struct {
	name    string
	payload string
}

// NewCredential create client credential
func NewCredential(payload string) *Credential {
	idx := strings.Index(payload, ":")
	if idx != -1 {
		authName := payload[:idx]
		idx++
		authPayload := payload[idx:]
		return &Credential{
			name:    authName,
			payload: authPayload,
		}
	}
	return &Credential{name: "none"}
}

// Payload client credential payload
func (c *Credential) Payload() string {
	return c.payload
}

// Name client credential name
func (c *Credential) Name() string {
	return c.name
}

// Authenticate finds an authentication way in `auths` and authenticates the Object.
//
// If `auths` is nil or empty, It returns true, It think that authentication is not required.
func Authenticate(auths map[string]Authentication, hf *frame.HandshakeFrame) (metadata.M, error) {
	if auths == nil || len(auths) <= 0 {
		return metadata.M{}, nil
	}

	if hf == nil {
		return metadata.M{}, errors.New("handshake frame cannot be nil")
	}

	auth, ok := auths[hf.AuthName]
	if !ok {
		return metadata.M{}, errors.New("authentication not found: " + hf.AuthName)
	}

	return auth.Authenticate(hf.AuthPayload)
}
