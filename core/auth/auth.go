package auth

import "strings"

var (
	auths = make(map[string]Authentication)
)

// Authentication for server
type Authentication interface {
	// Init authentication initialize arguments
	Init(args ...string)
	// Authenticate authentication client's credential
	Authenticate(payload string) bool
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
